package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSequenceEditorReadsOriginalAndScopedEnhancements(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	g := newGateway(config{gateway: "/app/clash-for-fnos", profilesFile: filepath.Join(root, "profiles.json"), profileDir: filepath.Join(root, "profiles")})
	original := "rules: [\"DOMAIN,test,DIRECT\"]\nproxies: [{name: Node, type: trojan, server: server.test, port: 443, password: fixture}]\nproxy-groups: [{name: Group, type: select, proxies: [Node]}]\n"
	imported := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]string{"Name": "A", "Content": original})
	g.ServeHTTP(imported, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/profiles/import", strings.NewReader(string(payload))))
	if imported.Code != http.StatusCreated {
		t.Fatal(imported.Body.String())
	}
	var item profile
	if err := json.Unmarshal(imported.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	extension := "prepend: [\"DOMAIN,custom,DIRECT\"]\nappend: []\ndelete: []\n"
	if err := os.WriteFile(filepath.Join(root, "profiles", item.ID+".rules.yaml"), []byte(extension), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"rules", "proxies", "groups"} {
		response := httptest.NewRecorder()
		g.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/profiles/"+item.ID+"/extensions/"+kind+"/editor", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", kind, response.Code, response.Body.String())
		}
		var data struct {
			Base       map[string]any    `json:"base"`
			Content    string            `json:"content"`
			Customized bool              `json:"customized"`
			Sequences  map[string]string `json:"sequences"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Sequences["rules"] != extension || len(data.Base["rules"].([]any)) != 1 || data.Base["rules"].([]any)[0] != "DOMAIN,test,DIRECT" {
			t.Fatalf("wrong source/scoped enhancements: %+v", data)
		}
		if data.Customized != (kind == "rules") {
			t.Fatalf("wrong customized flag %s", kind)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "profiles", item.ID+".yaml"))
	if err != nil || string(raw) != original {
		t.Fatal("editor changed subscription source")
	}
	// An undownloaded remote profile must still allow editing enhancements.
	if err := os.Remove(filepath.Join(root, "profiles", item.ID+".yaml")); err != nil {
		t.Fatal(err)
	}
	undownloaded := httptest.NewRecorder()
	g.ServeHTTP(undownloaded, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/profiles/"+item.ID+"/extensions/groups/editor", nil))
	if undownloaded.Code != http.StatusOK || !strings.Contains(undownloaded.Body.String(), "订阅尚未下载") {
		t.Fatal(undownloaded.Body.String())
	}
	missing := httptest.NewRecorder()
	g.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/profiles/missing/extensions/rules/editor", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatal(missing.Code)
	}
}
