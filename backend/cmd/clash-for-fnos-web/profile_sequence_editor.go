package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/chenpingonline/Clash-for-fnos/backend/internal/configyaml"
	"gopkg.in/yaml.v3"
)

// Read the original subscription separately from enhancements. Deletions must
// target original identities so downloading a subscription cannot erase edits.
func (g *gateway) handleProfileSequenceEditorAPI(w http.ResponseWriter, r *http.Request, id, kind string) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if kind != "rules" && kind != "proxies" && kind != "groups" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "增强类型不存在"})
		return true
	}
	g.profileMu.Lock()
	defer g.profileMu.Unlock()
	state, err := g.readProfiles()
	if err != nil || findProfile(&state, id) == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "配置不存在"})
		return true
	}
	base := map[string]any{}
	warning := ""
	raw, err := os.ReadFile(filepath.Join(g.config.profileDir, id+".yaml"))
	if os.IsNotExist(err) {
		warning = "订阅尚未下载，请先更新订阅以查看原始内容"
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return true
	} else if err = yaml.Unmarshal(raw, &base); err != nil || base == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "订阅 YAML 无效，无法读取原始内容"})
		return true
	}
	extensions, err := g.readProfileExtensions(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return true
	}
	content, customized := extensions[kind]
	if !customized {
		content = configyaml.DefaultExtension(kind)
	}
	sequences := map[string]string{}
	for _, key := range []string{"rules", "proxies", "groups"} {
		sequences[key] = extensions[key]
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": content, "customized": customized, "base": base, "sequences": sequences, "warning": warning})
	return true
}
