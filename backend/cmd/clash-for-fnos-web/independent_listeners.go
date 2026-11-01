package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Resolve the profile on the host; clients never supply configuration paths or
// raw YAML. Serialize saves against both subscription updates and network saves.
func (g *gateway) manageIndependentListeners(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	g.profileMu.Lock()
	defer g.profileMu.Unlock()
	state, err := g.readProfiles()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id := r.URL.Query().Get("profileId")
	if id == "" && state.Current != nil {
		id = *state.Current
	}
	scope := "runtime"
	current := state.Current == nil
	content := []byte{}
	if id != "" {
		item := findProfile(&state, id)
		if item == nil {
			writeJSON(w, 404, map[string]string{"error": "配置不存在"})
			return
		}
		scope = "profile:" + id
		current = state.Current != nil && *state.Current == id
		content, err = os.ReadFile(filepath.Join(g.config.profileDir, id+".yaml"))
		if err == nil {
			content, err = g.independentProfileContent(item, content)
		}
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
	}
	input := map[string]any{"scope": scope, "content": string(content)}
	if r.Method == http.MethodPut {
		if !decodeJSONBody(w, r, &input) {
			return
		}
		if input == nil {
			writeJSON(w, 400, map[string]string{"error": "独立代理入口格式无效"})
			return
		}
		// Host-resolved fields cannot be overridden by the request body.
		input["scope"], input["content"] = scope, string(content)
		if current {
			payload := map[string]any{"listenerScope": scope, "independentListeners": input["entries"], "independentListenersEnabled": input["enabled"], "assignUnassigned": input["assignUnassigned"]}
			body, _ := json.Marshal(payload)
			request := r.Clone(r.Context())
			request.Body = io.NopCloser(bytes.NewReader(body))
			request.ContentLength = int64(len(body))
			g.updateNetworkSettings(w, request)
			return
		}
	}
	g.networkMu.Lock()
	defer g.networkMu.Unlock()
	path := "/independent/read"
	if r.Method == http.MethodPut {
		path = "/independent/save"
	}
	var status map[string]any
	if err := g.helperJSON(r.Context(), http.MethodPost, path, input, &status, 30*time.Second); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	status["current"], status["profileId"], status["savedOnly"] = current, id, !current
	writeJSON(w, 200, status)
}
