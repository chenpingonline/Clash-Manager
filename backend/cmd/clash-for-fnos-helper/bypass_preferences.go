package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// User intent is separate from the temporary recovery journal. Lifecycle cleanup
// removes interception rules, but must not erase an explicitly saved keep intent.
type bypassPreferences struct {
	Schema      int             `json:"schema"`
	Mode        string          `json:"mode"`
	Interface   bypassInterface `json:"interface"`
	Enabled     bool            `json:"enabled"`
	KeepEnabled bool            `json:"keepEnabled"`
	LastError   string          `json:"lastError,omitempty"`
}

func (h *helper) bypassPreferencesPath() string {
	return filepath.Join(h.config.etcDir, "bypass-preferences.json")
}
func (h *helper) readBypassPreferences() (*bypassPreferences, error) {
	raw, err := os.ReadFile(h.bypassPreferencesPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p bypassPreferences
	if json.Unmarshal(raw, &p) != nil || p.Schema != 1 || (p.Mode != "redir" && p.Mode != "tproxy") || !validBypassInterface(p.Interface) {
		return nil, errors.New("接管偏好记录无效，请重新选择接管配置")
	}
	return &p, nil
}
func (h *helper) saveBypassPreferences(p *bypassPreferences) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWrite(h.bypassPreferencesPath(), raw, 0600)
}
func (h *helper) updateBypassKeep(body map[string]any) (map[string]any, error) {
	keep, ok := body["keepEnabled"].(bool)
	if !ok {
		return nil, fail(400, "保持启用必须为布尔值")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bypassMu.Lock()
	defer h.bypassMu.Unlock()
	p, err := h.readBypassPreferences()
	if err != nil {
		return nil, err
	}
	// Upgrade existing active installations without silently opting them in.
	if p == nil {
		s, e := h.readBypassState()
		if e != nil {
			return nil, e
		}
		if s == nil {
			return nil, fail(409, "请先成功启用一次接管配置")
		}
		p = &bypassPreferences{Schema: 1, Mode: s.Mode, Interface: s.Interface, Enabled: true}
	}
	p.KeepEnabled = keep
	p.LastError = ""
	if err = h.saveBypassPreferences(p); err != nil {
		return nil, err
	}
	h.bypassRestoreError = ""
	if keep && p.Enabled {
		h.scheduleBypassRestore(0)
	}
	return map[string]any{"ok": true, "keepEnabled": keep}, nil
}
func (h *helper) stopBypass(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bypassMu.Lock()
	defer h.bypassMu.Unlock()
	p, err := h.readBypassPreferences()
	if err != nil {
		return err
	}
	if p == nil {
		previous, readErr := h.readBypassState()
		if readErr != nil {
			return readErr
		}
		if previous != nil {
			p = &bypassPreferences{Schema: 1, Mode: previous.Mode, Interface: previous.Interface}
		}
	}

	if p != nil {
		p.Enabled = false
		p.LastError = ""
		if err = h.saveBypassPreferences(p); err != nil {
			return err
		}
	}
	h.bypassRestoreError = ""
	return h.cleanupBypassLocked(ctx, 0)
}
func (h *helper) bypassProcess() *processInfo {
	if h.bypassSystem != nil && h.bypassSystem.process != nil {
		return h.bypassSystem.process()
	}
	return h.managedProcess()
}
func (h *helper) bypassInterfaces() []bypassInterface {
	if h.bypassSystem != nil && h.bypassSystem.interfaces != nil {
		return h.bypassSystem.interfaces()
	}
	return monitorInterfaces()
}
func (h *helper) probeBypassPort(ctx context.Context, address string) error {
	if h.bypassSystem != nil && h.bypassSystem.probe != nil {
		return h.bypassSystem.probe(ctx, address)
	}
	connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", address)
	if err == nil {
		connection.Close()
	}
	return err
}
func (h *helper) readBypassLiveConfig(ctx context.Context, raw []byte) (map[string]any, error) {
	if h.bypassSystem != nil && h.bypassSystem.liveConfig != nil {
		return h.bypassSystem.liveConfig(ctx, raw)
	}
	controller, secret, _ := parseController(string(raw))
	host, port, err := net.SplitHostPort(controller)
	if err != nil {
		return nil, errors.New("无法读取托管内核的 Controller 地址")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		controller = net.JoinHostPort("127.0.0.1", port)
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+controller+"/configs", nil)
	if err != nil {
		return nil, errors.New("无法读取托管内核运行配置")
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("托管内核运行配置暂不可读取")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("托管内核运行配置暂不可读取")
	}
	var result map[string]any
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result) != nil {
		return nil, errors.New("托管内核运行配置无效")
	}
	return result, nil
}
func validateBypassRestore(raw []byte, p *bypassPreferences, live map[string]any) error {
	if yamlNestedBoolean(string(raw), "tun", "enable", false) {
		return errors.New("TUN 已开启，未恢复独立接管规则")
	}
	port := yamlInteger(string(raw), p.Mode+"-port", 0)
	if port <= 0 || !yamlBoolean(string(raw), "allow-lan", false) || proxyBindAddressFromConfig(string(raw)) != p.Interface.Address {
		return errors.New("透明代理端口或监听地址已变化，未恢复接管规则")
	}
	tun, ok := live["tun"].(map[string]any)
	enabled, known := tun["enable"].(bool)
	actual, present := live[p.Mode+"-port"].(float64)
	if !ok || !known || enabled || !present || actual != float64(port) {
		return errors.New("运行中的透明代理端口或 TUN 状态尚未按预期生效")
	}
	return nil
}
func (h *helper) scheduleBypassRestore(pid int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		h.reconcileBypass(ctx, pid)
	}()
}
func (h *helper) reconcileBypass(ctx context.Context, pid int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bypassMu.Lock()
	defer h.bypassMu.Unlock()
	p, err := h.readBypassPreferences()
	if err != nil {
		h.bypassRestoreError = err.Error()
		return
	}
	if p == nil || !p.Enabled || !p.KeepEnabled || ctx.Err() != nil {
		return
	}
	proc := h.bypassProcess()
	if h.readMode() != "managed" || proc == nil || (pid != 0 && proc.PID != pid) {
		return
	}
	for _, tx := range h.transactions {
		if tx.Activation != "" {
			return
		}
	}
	if err = h.restoreBypassLocked(ctx, p, proc.PID); err != nil {
		p.LastError = "自动恢复接管失败：" + regexp.MustCompile(`[a-f0-9]{32}`).ReplaceAllString(err.Error(), "[规则标记]")
		if len(p.LastError) > 600 {
			p.LastError = "自动恢复接管失败，请查看内核与网络配置"
		}
	} else {
		p.LastError = ""
	}
	h.bypassRestoreError = p.LastError
	if h.saveBypassPreferences(p) != nil {
		h.bypassRestoreError = "无法保存接管恢复状态，请检查存储权限与空间"
	}
}

// Caller holds mu then bypassMu. No network configuration is changed on restore.
func (h *helper) restoreBypassLocked(ctx context.Context, p *bypassPreferences, pid int) error {
	raw, err := os.ReadFile(h.config.managedConfig)
	if err != nil {
		return errors.New("托管配置不可读取")
	}
	var live map[string]any
	deadline := time.Now().Add(5 * time.Second)
	for {
		live, err = h.readBypassLiveConfig(ctx, raw)
		if err == nil {
			err = validateBypassRestore(raw, p, live)
		}
		if err == nil {
			break
		}
		if time.Now().After(deadline) || yamlNestedBoolean(string(raw), "tun", "enable", false) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	s, err := h.readBypassState()
	if err != nil {
		return err
	}
	if s != nil {
		// Repeated lifecycle signals may find this process already configured.
		tables, e := h.bypassOS().run(ctx, "", "nft", "-j", "list", "tables")
		if e != nil {
			return e
		}
		exists, owner, e := h.bypassOwner(ctx, tables)
		if e != nil {
			return e
		}
		if exists && owner == "clash-manager:"+s.Token && s.CorePID == pid && s.Mode == p.Mode && s.Interface == p.Interface && s.Port == yamlInteger(string(raw), p.Mode+"-port", 0) {
			return nil
		}
		if err = h.cleanupBypassLocked(ctx, 0); err != nil {
			return err
		}
	}
	plan, err := h.bypassPreflight(ctx, map[string]any{"mode": p.Mode, "interface": p.Interface.Name, "address": p.Interface.Address})
	if err != nil {
		return err
	}
	if plan.Interface != p.Interface {
		return errors.New("接入网卡或网段已变化，请重新选择后启用")
	}
	if plan.CorePID != pid {
		return errors.New("内核进程已变化，请重新启用")
	}
	if err = h.probeBypassPort(ctx, net.JoinHostPort(plan.Interface.Address, fmt.Sprint(plan.Port))); err != nil {
		return errors.New("透明代理端口尚未运行")
	}
	_, err = h.installBypassPlan(ctx, plan)
	return err
}
func (h *helper) attachBypassPreferences(status map[string]any) map[string]any {
	if h.bypassRestoreError != "" && status["error"] == nil {
		status["error"] = h.bypassRestoreError
	}
	p, err := h.readBypassPreferences()
	if err != nil {
		if status["error"] == nil {
			status["error"] = err.Error()
		}
		return status
	}
	if p != nil {
		status["selection"] = map[string]any{"mode": p.Mode, "interface": p.Interface}
		status["keepEnabled"] = p.KeepEnabled
		status["enabled"] = p.Enabled
		if p.LastError != "" && status["error"] == nil {
			status["error"] = p.LastError
		}
	}
	return status
}
