package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/logstore"
	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomolog"
	"github.com/chenpingonline/Clash-Manager/backend/internal/privileged"
)

func (g *gateway) logSettingsFile() string {
	return logstore.SettingsPath(filepath.Dir(g.config.settingsFile))
}
func (g *gateway) logSettingsStatus(ctx context.Context) (map[string]any, error) {
	settings, err := logstore.LoadSettings(g.logSettingsFile())
	if err != nil {
		return nil, err
	}
	history, err := g.logs.Stats()
	if err != nil {
		return nil, err
	}
	core := map[string]any{"available": false, "size": 0, "maxBytes": int64(settings.Core.MaxMiB) * logstore.MiB}
	if err := (privileged.Client{SocketPath: g.config.privilegedSocket}).DoJSON(ctx, http.MethodGet, "/logs/status", nil, &core, 3*time.Second); err != nil {
		core["available"] = false
		core["error"] = "Helper 不可用，Core 日志设置将在助手启动后生效"
	}
	return map[string]any{"settings": settings, "history": history, "core": core}, nil
}
func (g *gateway) handleLogSettings(w http.ResponseWriter, r *http.Request, path string) bool {
	if path != "/api/logs/settings" && path != "/api/logs/core" {
		return false
	}
	if path == "/api/logs/core" {
		if r.Method == http.MethodGet {
			g.readCoreLogs(w, r)
			return true
		}
		if r.Method != http.MethodDelete {
			writeJSON(w, 405, map[string]string{"error": "Method not allowed"})
			return true
		}
		var status map[string]any
		err := (privileged.Client{SocketPath: g.config.privilegedSocket}).DoJSON(r.Context(), http.MethodPost, "/logs/clear", map[string]any{}, &status, 5*time.Second)
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, 200, status)
		}
		return true
	}
	g.logSettingsMu.Lock()
	defer g.logSettingsMu.Unlock()
	if r.Method == http.MethodPut {
		body, ok := readLimitedBody(w, r, 16*1024)
		if !ok {
			return true
		}
		var settings logstore.Settings
		if json.Unmarshal(body, &settings) != nil {
			writeJSON(w, 400, map[string]string{"error": "JSON 格式错误"})
			return true
		}
		if err := settings.Validate(); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return true
		}
		if err := logstore.SaveSettings(g.logSettingsFile(), settings); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return true
		}
		if err := g.logs.Configure(settings.History, settings.SaveLevel); err != nil {
			writeJSON(w, 500, map[string]string{"error": "设置已保存，但日志清理失败: " + err.Error()})
			return true
		}
	} else if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "Method not allowed"})
		return true
	}
	status, err := g.logSettingsStatus(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
	} else {
		writeJSON(w, 200, status)
	}
	return true
}

func (g *gateway) readCoreLogs(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	limit := values.Get("limit")
	if limit == "" {
		limit = "800"
	}
	n, err := strconv.Atoi(limit)
	if err != nil || n < 1 || n > 2000 {
		writeJSON(w, 400, map[string]string{"error": "日志行数须为 1–2000"})
		return
	}
	// Forward only supported query fields; the helper owns the fixed file path.
	params := url.Values{"limit": {limit}, "search": {values.Get("search")}, "cursor": {values.Get("cursor")}}
	var page logstore.RawPage
	err = (privileged.Client{SocketPath: g.config.privilegedSocket}).DoJSON(r.Context(), http.MethodGet, "/logs/core?"+params.Encode(), nil, &page, 12*time.Second)
	if err != nil {
		code := 503
		if err.Error() == logstore.ErrCursorExpired.Error() {
			code = 409
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, page)
}
func (g *gateway) logHistory(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	level := values.Get("level")
	if level == "" {
		level = "info"
	}
	switch level {
	case "debug", "info", "warning", "error":
	default:
		writeJSON(w, 400, map[string]string{"error": "日志级别无效"})
		return
	}
	query := logstore.Query{Level: level, Search: values.Get("search"), Cursor: values.Get("cursor"), Limit: 2000}
	if values.Get("limit") != "" {
		query.Limit = mihomolog.ParseLimit(values.Get("limit"))
	}
	for key, target := range map[string]*time.Time{"from": &query.From, "to": &query.To} {
		if values.Get(key) != "" {
			value, err := time.Parse(time.RFC3339Nano, values.Get(key))
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "日志查询日期无效"})
				return
			}
			*target = value
		}
	}
	if !query.From.IsZero() && !query.To.IsZero() && !query.From.Before(query.To) {
		writeJSON(w, 400, map[string]string{"error": "开始日期须早于结束日期"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := g.logs.Query(ctx, query)
	if err != nil {
		status := 500
		if errors.Is(err, logstore.ErrCursorExpired) {
			status = 409
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
	} else {
		writeJSON(w, 200, page)
	}
}
