package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestListenerProfileAPIResolvesScopeAndIsolatesInactiveSaves(t *testing.T) {
	root := t.TempDir()
	var calls atomic.Int32
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input["scope"] != "profile:b" || !strings.Contains(input["content"].(string), "B-node") || strings.Contains(input["content"].(string), "untrusted") {
			t.Errorf("scope/content not host resolved: %v", input)
		}
		if r.URL.Path != "/independent/read" && r.URL.Path != "/independent/save" {
			t.Errorf("inactive profile touched runtime: %s", r.URL.Path)
		}
		writeJSON(w, 200, map[string]any{"enabled": true, "entries": []any{}, "targets": []string{"B-node"}})
	}))
	cfg := config{profileDir: root, profilesFile: filepath.Join(root, "profiles.json"), privilegedSocket: socket}
	g := newGateway(cfg)
	a := "a"
	if err := g.writeProfiles(profileState{Current: &a, Items: []*profile{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.yaml"), []byte("proxies: [{name: B-node}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(method, "/api/independent-listeners?profileId=b", strings.NewReader(`{"scope":"profile:a","content":"untrusted","entries":[],"enabled":true}`)))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"current":false`) || !strings.Contains(w.Body.String(), `"savedOnly":true`) {
			t.Fatalf("%s: %d %s", method, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/independent-listeners?profileId=unknown", nil))
	if w.Code != 404 || calls.Load() != 2 {
		t.Fatalf("unknown profile reached helper: %d calls=%d", w.Code, calls.Load())
	}
	nullSave := httptest.NewRecorder()
	g.ServeHTTP(nullSave, httptest.NewRequest(http.MethodPut, "/api/independent-listeners?profileId=b", strings.NewReader("null")))
	if nullSave.Code != 400 || calls.Load() != 2 {
		t.Fatalf("null request reached helper: %d calls=%d", nullSave.Code, calls.Load())
	}
	state, err := g.readProfiles()
	if err != nil || state.Current == nil || *state.Current != "a" {
		t.Fatal("inactive save changed active profile", err)
	}
}

func TestCurrentListenerProfileSaveUsesNetworkTransaction(t *testing.T) {
	root := t.TempDir()
	var updates, activates, commits atomic.Int32
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network/update":
			updates.Add(1)
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["listenerScope"] != "profile:a" || input["independentListenersEnabled"] != false {
				t.Errorf("bad transaction input: %v", input)
			}
			writeJSON(w, 200, map[string]any{"txId": "listeners", "settings": map[string]any{}})
		case "/config/activate":
			activates.Add(1)
			writeJSON(w, 200, map[string]any{"method": "saved-only"})
		case "/config/commit":
			commits.Add(1)
			writeJSON(w, 200, map[string]any{"ok": true})
		case "/status":
			writeJSON(w, 200, map[string]any{})
		case "/system/proxy-environment/sync":
			writeJSON(w, 200, map[string]any{})
		default:
			t.Errorf("unexpected helper call %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	cfg := config{profileDir: root, profilesFile: filepath.Join(root, "profiles.json"), settingsFile: filepath.Join(root, "settings.json"), privilegedSocket: socket}
	g := newGateway(cfg)
	a := "a"
	_ = g.writeProfiles(profileState{Current: &a, Items: []*profile{{ID: "a", Name: "A"}}})
	_ = os.WriteFile(filepath.Join(root, "a.yaml"), []byte("proxies: []\n"), 0600)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/independent-listeners?profileId=a", strings.NewReader(`{"enabled":false,"entries":[]}`)))
	if w.Code != 200 || updates.Load() != 1 || activates.Load() != 1 || commits.Load() != 1 {
		t.Fatalf("save bypassed transaction: %d %s", w.Code, w.Body.String())
	}
}
