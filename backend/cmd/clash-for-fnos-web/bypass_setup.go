package main

import (
	"context"
	"errors"
	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
	"net/http"
	"time"
)

// Apply the validated Core config before installing interception rules.
func (g *gateway) configureBypass(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeJSON(w, 403, map[string]string{"error": "拒绝跨站请求"})
		return
	}
	var body map[string]any
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body["confirmed"] != true {
		writeJSON(w, 400, map[string]string{"error": "请先确认接管范围"})
		return
	}
	g.networkMu.Lock()
	defer g.networkMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	var previous map[string]any
	if err := g.helperJSON(ctx, http.MethodGet, "/config/active-raw", nil, &previous, 5*time.Second); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	var prepared map[string]any
	if err := g.helperJSON(ctx, http.MethodPost, "/network/bypass/prepare", body, &prepared, 25*time.Second); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	txID, ok := prepared["txId"].(string)
	if !ok || txID == "" {
		writeJSON(w, 502, map[string]string{"error": "接管配置事务无效"})
		return
	}
	rollback := func(cause error) {
		recovery, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		cleanupErr := g.helperJSON(recovery, http.MethodPost, "/network/bypass/stop", map[string]any{}, nil, 20*time.Second)
		configErr := g.helperJSON(recovery, http.MethodPost, "/config/rollback", map[string]any{"txId": txID}, nil, 10*time.Second)
		if configErr == nil {
			if raw, ok := previous["content"].(string); ok {
				configErr = g.applyConfig(recovery, []byte(raw))
			} else {
				configErr = errors.New("旧配置不可用")
			}
		}
		err := errors.Join(cleanupErr, configErr)
		message := "接管失败，已恢复原配置: " + cause.Error()
		if err != nil {
			message = "接管失败，恢复不完整: " + cause.Error() + "；" + err.Error()
		}
		writeJSON(w, 502, map[string]string{"error": message})
	}
	var activation map[string]any
	if err := g.helperJSON(ctx, http.MethodPost, "/config/activate", map[string]any{"txId": txID}, &activation, 15*time.Second); err != nil {
		rollback(err)
		return
	}
	effective, ok := prepared["effectiveContent"].(string)
	if !ok || activation["method"] != "hot-reload" {
		rollback(errors.New("托管内核未处于运行状态"))
		return
	}
	if err := g.applyConfig(ctx, []byte(effective)); err != nil {
		rollback(err)
		return
	}
	if err := g.waitController(ctx, 10*time.Second); err != nil {
		rollback(err)
		return
	}
	// Confirm the listener and TUN state in Core, not just the saved file.
	live, err := mihomoJSON(ctx, &mihomo.Client{SettingsFile: g.config.settingsFile}, "/configs")
	if err != nil {
		rollback(err)
		return
	}
	tun, present := live["tun"].(map[string]any)
	enabled, known := tun["enable"].(bool)
	settings, _ := prepared["settings"].(map[string]any)
	mode, _ := body["mode"].(string)
	portSetting, _ := settings[mode].(map[string]any)
	expected, hasExpected := portSetting["port"].(float64)
	actual, hasActual := live[mode+"-port"].(float64)
	if !present || !known || enabled || !hasExpected || !hasActual || actual != expected {
		rollback(errors.New("运行中的透明代理端口或 TUN 状态未按预期生效"))
		return
	}
	var result map[string]any
	if err := g.helperJSON(ctx, http.MethodPost, "/network/bypass/apply", body, &result, 25*time.Second); err != nil {
		rollback(err)
		return
	}
	if err := g.helperJSON(ctx, http.MethodPost, "/config/commit", map[string]any{"txId": txID}, nil, 5*time.Second); err != nil {
		rollback(err)
		return
	}
	result["message"] = "IPv4 接管已配置，设备流量是否经过仍需实际连接验证"
	writeJSON(w, 200, result)
}
