package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Bind edits to both the file and its contents. Subscription updates or a
// profile switch must not silently overwrite changes made since opening it.
func configEditorRevision(active map[string]any) string {
	body, _ := json.Marshal([]any{active["path"], active["content"]})
	return contentETag(body)
}

func (g *gateway) handleConfigEditor(w http.ResponseWriter, r *http.Request) {
	var edit struct {
		Content  string `json:"content"`
		Revision string `json:"revision"`
	}
	if r.Method == http.MethodPut {
		if !decodeJSONBody(w, r, &edit) {
			return
		}
		if strings.TrimSpace(edit.Content) == "" || strings.ContainsRune(edit.Content, 0) || edit.Revision == "" {
			writeJSON(w, 400, map[string]string{"error": "配置内容或版本标识无效"})
			return
		}
	}
	// Match the lock order used by subscription activation and network settings.
	g.networkMu.Lock()
	defer g.networkMu.Unlock()
	g.configMu.Lock()
	defer g.configMu.Unlock()
	_, active, err := g.activeStartupConfig(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "读取启动配置失败: " + err.Error()})
		return
	}
	if active["mode"] != "managed" {
		writeJSON(w, 403, map[string]string{"error": "仅支持编辑 Manager 托管配置；外部 Core 请在其配置来源中修改"})
		return
	}
	if r.Method == http.MethodGet {
		active["revision"] = configEditorRevision(active)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, active)
		return
	}
	if edit.Revision != configEditorRevision(active) {
		writeJSON(w, 409, map[string]string{"error": "配置已被更新或切换。请保留修改内容，重新打开编辑器后再合并保存"})
		return
	}
	result, err := g.syncStartupConfig(r.Context(), []byte(edit.Content))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	result["ok"] = true
	writeJSON(w, 200, result)
}
