package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/chenpingonline/Clash-Manager/backend/internal/logstore"
)

func (g *gateway) logDisplaySettings(w http.ResponseWriter, r *http.Request) {
	g.logSettingsMu.Lock()
	defer g.logSettingsMu.Unlock()
	file := logstore.DisplaySettingsPath(filepath.Dir(g.config.settingsFile))
	switch r.Method {
	case http.MethodGet:
		value, err := logstore.LoadDisplaySettings(file)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, value)
	case http.MethodPut:
		body, ok := readLimitedBody(w, r, 1024)
		if !ok {
			return
		}
		var value logstore.DisplaySettings
		if json.Unmarshal(body, &value) != nil {
			writeJSON(w, 400, map[string]string{"error": "JSON 格式错误"})
			return
		}
		if err := value.Validate(); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := logstore.SaveDisplaySettings(file, value); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, value)
	default:
		writeJSON(w, 405, map[string]string{"error": "Method not allowed"})
	}
}
