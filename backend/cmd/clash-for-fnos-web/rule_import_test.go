package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/ruleimport"
)

func importFixture(t *testing.T) (*gateway, string, string) {
	t.Helper()
	root := t.TempDir()
	g := newGateway(config{gateway: "/app/clash-for-fnos", profilesFile: filepath.Join(root, "profiles.json"), profileDir: filepath.Join(root, "profiles")})
	response := httptest.NewRecorder()
	g.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/profiles/import", strings.NewReader(`{"name":"A","content":"proxies: []\nproxy-groups: []\nrules: [\"MATCH,DIRECT\"]\n"}`)))
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	var item profile
	_ = json.Unmarshal(response.Body.Bytes(), &item)
	return g, item.ID, root
}
func importPost(g *gateway, id, operation string, payload any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(payload)
	response := httptest.NewRecorder()
	g.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/profiles/"+id+"/rule-import/"+operation, strings.NewReader(string(raw))))
	return response
}
func previewFixture(t *testing.T, g *gateway, id, content string) ruleImportPreview {
	t.Helper()
	response := importPost(g, id, "preview", ruleImportInput{Source: "text", Content: content, Options: ruleimport.Options{Name: "Local", Policy: "DIRECT"}})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var p ruleImportPreview
	if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestRuleImportPreviewIsReadOnlyAndSavePreservesOriginal(t *testing.T) {
	g, id, root := importFixture(t)
	original, _ := os.ReadFile(filepath.Join(root, "profiles", id+".yaml"))
	path := filepath.Join(root, "profiles", id+".override.yaml")
	if err := os.WriteFile(path, []byte("dns: {enable: true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p := previewFixture(t, g, id, "example.com\n+.example.net")
	if len(p.Errors) != 0 || len(p.Providers) != 1 {
		t.Fatal(p.Errors)
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", id+".rules.yaml")); !os.IsNotExist(err) {
		t.Fatal("preview wrote rules")
	}
	response := importPost(g, id, "save", map[string]any{"token": p.ID})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	after, _ := os.ReadFile(filepath.Join(root, "profiles", id+".yaml"))
	if string(after) != string(original) {
		t.Fatal("source mutated")
	}
	override, _ := os.ReadFile(path)
	if !strings.Contains(string(override), "dns:") || !strings.Contains(string(override), "Local:") {
		t.Fatal(string(override))
	}
	rules, _ := os.ReadFile(filepath.Join(root, "profiles", id+".rules.yaml"))
	if !strings.Contains(string(rules), "RULE-SET,Local,DIRECT") {
		t.Fatal(string(rules))
	}
	response = importPost(g, id, "save", map[string]any{"token": p.ID})
	if response.Code != 409 {
		t.Fatal("consumed token accepted")
	}
}
func TestRuleImportRejectsStalePreviewAndWrongProfile(t *testing.T) {
	g, id, root := importFixture(t)
	p := previewFixture(t, g, id, "example.com")
	response := importPost(g, "other", "save", map[string]any{"token": p.ID})
	if response.Code != 409 {
		t.Fatal(response.Code)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", id+".rules.yaml"), []byte("prepend: [\"DOMAIN,other.example,DIRECT\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	response = importPost(g, id, "save", map[string]any{"token": p.ID})
	if response.Code != 409 {
		t.Fatal(response.Code, response.Body.String())
	}
	body, _ := os.ReadFile(filepath.Join(root, "profiles", id+".rules.yaml"))
	if !strings.Contains(string(body), "other.example") {
		t.Fatal("concurrent enhancement lost")
	}
}
func TestRuleImportRequiresAcknowledgmentAndBlocksErrors(t *testing.T) {
	g, id, _ := importFixture(t)
	p := previewFixture(t, g, id, "DOMAIN-SUFFIX,example.com\nUSER-AGENT,foo")
	if len(p.Warnings) != 1 {
		t.Fatal(p.Warnings)
	}
	response := importPost(g, id, "save", map[string]any{"token": p.ID})
	if response.Code != 400 {
		t.Fatal(response.Code)
	}
	response = importPost(g, id, "save", map[string]any{"token": p.ID, "acknowledge": true})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	p = previewFixture(t, g, id, "rules: [\"DOMAIN-SUFFIX,foo,missing\"]")
	if len(p.Errors) == 0 {
		t.Fatal("missing policy not previewed")
	}
	response = importPost(g, id, "save", map[string]any{"token": p.ID, "acknowledge": true})
	if response.Code != 400 {
		t.Fatal(response.Code)
	}
}
func TestRuleImportURLAndExactPreviewSave(t *testing.T) {
	g, id, root := importFixture(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte("payload: ['+.example.com']"))
	}))
	defer server.Close()
	response := importPost(g, id, "preview", ruleImportInput{Source: "url", Options: ruleimport.Options{Name: "Remote", Policy: "DIRECT", URL: server.URL + "/domain.txt"}})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var p ruleImportPreview
	_ = json.Unmarshal(response.Body.Bytes(), &p)
	if p.Providers[0].Format != "yaml" || requests != 1 {
		t.Fatal(p.Providers, requests)
	}
	response = importPost(g, id, "save", map[string]any{"token": p.ID})
	if response.Code != 200 || requests != 1 {
		t.Fatal(response.Code, requests, response.Body.String())
	}
	body, _ := os.ReadFile(filepath.Join(root, "profiles", id+".override.yaml"))
	if !strings.Contains(string(body), server.URL) || !strings.Contains(string(body), "format: yaml") {
		t.Fatal(string(body))
	}
}
func TestRuleImportFileTransactionDoesNotLeavePartialEnhancements(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.yaml")
	b := filepath.Join(root, "b.yaml")
	if err := os.WriteFile(a, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(b, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeRuleImportFiles(map[string][]byte{a: []byte("new"), b: []byte("unwritable")}); err == nil {
		t.Fatal("directory target accepted")
	}
	after, _ := os.ReadFile(a)
	if string(after) != "old" {
		t.Fatal("partial write")
	}
}

func TestRuleImportSaveDoesNotReuseUnrelatedActiveJob(t *testing.T) {
	g, id, root := importFixture(t)
	p := previewFixture(t, g, id, "example.com")
	g.jobMu.Lock()
	g.activeJobs[id] = "active"
	g.profileJobs["active"] = &profileJob{ID: "active", ProfileID: id, State: "running", Operation: "update"}
	g.jobMu.Unlock()
	response := importPost(g, id, "save", map[string]any{"token": p.ID, "apply": true})
	if response.Code != 409 {
		t.Fatal(response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", id+".rules.yaml")); !os.IsNotExist(err) {
		t.Fatal("saved while another job was running")
	}
}
