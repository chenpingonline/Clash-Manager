package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

type tunInterfaceState struct {
	Name    string `json:"name"`
	Checked bool   `json:"checked"`
	Exists  bool   `json:"exists"`
	Up      bool   `json:"up"`
	TUN     bool   `json:"tun"`
	Reason  string `json:"reason,omitempty"`
}
type tunState struct {
	Requirements   *configyaml.TunRequirements `json:"requirements,omitempty"`
	Phase          string                      `json:"phase"`
	Message        string                      `json:"message"`
	DesiredEnabled *bool                       `json:"desiredEnabled,omitempty"`
	RuntimeEnabled *bool                       `json:"runtimeEnabled,omitempty"`
	Interface      *tunInterfaceState          `json:"interface,omitempty"`
	Capability     map[string]any              `json:"capability,omitempty"`
	Operation      tunOperationStatus          `json:"operation"`
}
type runtimeTunState struct {
	Enable *bool  `json:"enable"`
	Device string `json:"device"`
}

func readRuntimeTun(ctx context.Context, client *mihomo.Client) (runtimeTunState, error) {
	res, err := client.Do(ctx, http.MethodGet, "/configs", nil, 2*time.Second)
	if err != nil {
		return runtimeTunState{}, err
	}
	defer res.Body.Close()
	var data struct {
		Tun runtimeTunState `json:"tun"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&data); err != nil {
		return data.Tun, err
	}
	if data.Tun.Enable == nil {
		return data.Tun, fmt.Errorf("Mihomo 未返回 TUN 运行状态")
	}
	return data.Tun, nil
}
func (g *gateway) inspectRuntimeTun(ctx context.Context, device string) (tunInterfaceState, error) {
	var state tunInterfaceState
	if device == "" {
		return state, fmt.Errorf("Mihomo 未返回 TUN 网卡名称，无法确认网卡")
	}
	err := g.helperJSON(ctx, http.MethodGet, "/network/tun/interface?device="+url.QueryEscape(device), nil, &state, 2*time.Second)
	return state, err
}
func (g *gateway) waitVerifiedTun(ctx context.Context, enabled bool, timeout time.Duration, deviceHint ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &mihomo.Client{SettingsFile: g.config.settingsFile}
	var last error
	for {
		runtime, err := readRuntimeTun(ctx, client)
		if err == nil && *runtime.Enable == enabled {
			if runtime.Device == "" && !enabled && len(deviceHint) > 0 {
				runtime.Device = deviceHint[0]
			}
			if !enabled && runtime.Device == "" {
				return nil
			}
			iface, probeErr := g.inspectRuntimeTun(ctx, runtime.Device)
			if probeErr == nil && iface.Checked && (enabled && iface.Exists && iface.TUN && iface.Up || !enabled && !iface.Exists) {
				return nil
			}
			if probeErr != nil {
				last = probeErr
			} else {
				last = fmt.Errorf("TUN 网卡状态未按预期生效: %s (checked=%t exists=%t up=%t tun=%t %s)", runtime.Device, iface.Checked, iface.Exists, iface.Up, iface.TUN, iface.Reason)
			}
		} else if err != nil {
			last = err
		} else {
			last = fmt.Errorf("TUN 配置运行状态未按预期生效")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("确认 TUN 运行状态失败: %w", last)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func (g *gateway) readTunState(ctx context.Context) (s tunState) {
	defer func() {
		g.tunOperationMu.RLock()
		current := g.tunOperation
		g.tunOperationMu.RUnlock()
		if current.Active {
			s.Phase = "switching"
			s.Message = current.Message
			s.Operation = current
			s.RuntimeEnabled = nil
			s.Interface = nil
		}
	}()
	g.tunOperationMu.RLock()
	op := g.tunOperation
	g.tunOperationMu.RUnlock()
	s = tunState{Phase: "checking", Message: "正在检测 TUN 状态…", Operation: op}
	g.networkOperationMu.RLock()
	network, core := g.networkOperation, g.coreOperation
	g.networkOperationMu.RUnlock()
	if op.Active || network.Active || core.Active {
		s.Phase = "switching"
		s.Message = op.Message
		if network.Active {
			s.Message = network.Message
		}
		if !op.Active {
			if core.Active {
				s.Message = core.Message
			} else {
				s.Message = network.Message
			}
		}
		return s
	}
	var networkStatus map[string]any
	if err := g.helperJSON(ctx, http.MethodGet, "/network/status", nil, &networkStatus, 2*time.Second); err != nil {
		s.Phase = "unknown"
		s.Message = "暂时无法读取 TUN 状态，已保留原有设置"
		return s
	}
	settings, _ := networkStatus["settings"].(map[string]any)
	tun, _ := settings["tun"].(map[string]any)
	desired, ok := tun["enabled"].(bool)
	if !ok {
		s.Phase = "unknown"
		s.Message = "暂时无法读取 TUN 状态，已保留原有设置"
		return s
	}
	s.DesiredEnabled = &desired
	s.Capability, _ = networkStatus["tunCapability"].(map[string]any)
	if data, err := json.Marshal(networkStatus["tunRequirements"]); err == nil && string(data) != "null" {
		var req configyaml.TunRequirements
		if json.Unmarshal(data, &req) == nil {
			s.Requirements = &req
		}
	}
	if networkStatus["offline"] == true {
		s.Phase = "stopped"
		s.Message = "Core 已停止，TUN 开启设置已保留"
		if !desired {
			s.Message = "Core 已停止，TUN 已关闭"
		}
		return s
	}
	rt, err := readRuntimeTun(ctx, &mihomo.Client{SettingsFile: g.config.settingsFile})
	if err != nil {
		s.Phase = "unknown"
		s.Message = "暂时无法读取 TUN 状态，已保留原有设置"
		return s
	}
	s.RuntimeEnabled = rt.Enable
	if rt.Device != "" {
		iface, err := g.inspectRuntimeTun(ctx, rt.Device)
		if err == nil {
			s.Interface = &iface
		}
	}
	switch {
	case *rt.Enable != desired:
		s.Phase = "mismatch"
		s.Message = "TUN 配置与 Core 状态不一致，请重新检测或切换"
	case !desired:
		s.Phase = "disabled"
		s.Message = "TUN 已关闭"
	case s.Interface == nil || !s.Interface.Checked:
		s.Phase = "unknown"
		s.Message = "TUN 配置已开启，网卡状态暂时无法确认"
	case !s.Interface.Exists || !s.Interface.TUN || !s.Interface.Up:
		s.Phase = "mismatch"
		s.Message = "TUN 配置已开启，但实际网卡尚未就绪"
	default:
		s.Phase = "enabled"
		s.Message = "TUN 已开启，实际网卡已就绪"
	}
	return s
}
func (g *gateway) writeTunState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, g.readTunState(r.Context()))
}
func (g *gateway) publishTunState() {
	g.tunStateEventsMu.Lock()
	defer g.tunStateEventsMu.Unlock()
	for ch := range g.tunStateListeners {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
func (g *gateway) subscribeTunState() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	g.tunStateEventsMu.Lock()
	if g.tunStateListeners == nil {
		g.tunStateListeners = map[chan struct{}]struct{}{}
	}
	g.tunStateListeners[ch] = struct{}{}
	g.tunStateEventsMu.Unlock()
	return ch, func() { g.tunStateEventsMu.Lock(); delete(g.tunStateListeners, ch); g.tunStateEventsMu.Unlock() }
}
func (g *gateway) streamTunState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	events, unsubscribe := g.subscribeTunState()
	defer unsubscribe()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		state := g.readTunState(r.Context())
		data, err := json.Marshal(state)
		if err != nil {
			return
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-events:
		}
	}
}
