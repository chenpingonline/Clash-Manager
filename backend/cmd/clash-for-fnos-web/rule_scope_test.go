package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

func TestRuleChoicesFollowSubscriptionAcrossSwitchAndRestart(t *testing.T) {
	var mu sync.Mutex
	disabled := map[int]bool{0: false, 1: false}
	reloads, patches := 0, 0
	failApply := false
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/version":
			writeJSON(w, 200, map[string]string{"version": "test"})
		case "/configs":
			if failApply {
				http.Error(w, "failed reload", 500)
				return
			}
			reloads++
			disabled = map[int]bool{0: false, 1: false}
			w.WriteHeader(204)
		case "/rules":
			writeJSON(w, 200, map[string]any{"rules": []any{
				map[string]any{"index": 0, "type": "DomainSuffix", "payload": "example.com", "proxy": "DIRECT", "extra": map[string]bool{"disabled": disabled[0]}},
				map[string]any{"index": 1, "type": "DomainSuffix", "payload": "other.com", "proxy": "DIRECT", "extra": map[string]bool{"disabled": disabled[1]}},
			}})
		case "/rules/disable":
			patches++
			var patch map[int]bool
			_ = json.NewDecoder(r.Body).Decode(&patch)
			for i, value := range patch {
				disabled[i] = value
			}
			w.WriteHeader(204)
		default:
			writeJSON(w, 200, map[string]any{})
		}
	}))
	defer controller.Close()
	disk, candidate := "", ""
	helper := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/config/active-raw":
			writeJSON(w, 200, map[string]any{"mode": "managed", "content": disk, "path": "/test/config.yaml"})
		case "/config/compose", "/config/sync":
			var input struct{ Content string }
			_ = json.NewDecoder(r.Body).Decode(&input)
			if r.URL.Path == "/config/compose" {
				writeJSON(w, 200, map[string]string{"content": input.Content})
				return
			}
			candidate = input.Content
			writeJSON(w, 200, map[string]any{"txId": "test", "effectiveContent": candidate})
		case "/config/activate":
			disk = candidate
			writeJSON(w, 200, map[string]string{"method": "hot-reload"})
		default:
			writeJSON(w, 200, map[string]any{})
		}
	}))
	root := t.TempDir()
	cfg := config{settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: helper, profileDir: root, profilesFile: filepath.Join(root, "profiles.json"), managedConfigFile: filepath.Join(root, "config.yaml"), configMetaFile: filepath.Join(root, "meta.json"), backupDir: filepath.Join(root, "backups"), ruleStateFile: filepath.Join(root, "rule-state.json")}
	// Identical YAML must still reload on a subscription switch, so old runtime
	// disables cannot leak through the unchanged-config optimization.
	raw := "mixed-port: 7890\nrules: [\"DOMAIN-SUFFIX,example.com,DIRECT\",\"DOMAIN-SUFFIX,other.com,DIRECT\"]\n"
	a, b := &profile{ID: "a", Name: "A"}, &profile{ID: "b", Name: "B"}
	for _, id := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(root, id+".yaml"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := profileState{Items: []*profile{a, b}}
	g := newGateway(cfg)
	activate := func(item *profile) {
		t.Helper()
		if _, err := g.activateProfileLocked(context.Background(), &state, item, true, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := g.refreshRules(context.Background(), &mihomo.Client{SettingsFile: cfg.settingsFile}); err != nil {
			t.Fatal(err)
		}
	}
	toggle := func(scope string, index int, value bool, want int) {
		t.Helper()
		payload := "example.com"
		if index == 1 {
			payload = "other.com"
		}
		body, _ := json.Marshal(map[string]any{"scope": scope, "index": index, "type": "DomainSuffix", "payload": payload, "proxy": "DIRECT", "disabled": value})
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/rules/disable", strings.NewReader(string(body))))
		if w.Code != want {
			t.Fatalf("toggle: %d %s", w.Code, w.Body.String())
		}
	}
	check := func(first, second bool) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if disabled[0] != first || disabled[1] != second {
			t.Fatalf("wrong subscription state: %v", disabled)
		}
	}
	activate(a)
	toggle("profile:a", 0, true, 200)
	activate(b)
	check(false, false)
	mu.Lock()
	if reloads != 2 {
		t.Fatalf("identical subscription switch skipped reload: %d", reloads)
	}
	before := patches
	mu.Unlock()
	toggle("profile:a", 0, true, 409)
	mu.Lock()
	if patches != before {
		t.Fatal("stale subscription request patched the new Core")
	}
	mu.Unlock()
	toggle("profile:b", 1, true, 200)
	activate(a)
	check(true, false)
	// Restart both processes; the owner and each subscription's preferences survive.
	g = newGateway(cfg)
	mu.Lock()
	disabled = map[int]bool{0: false, 1: false}
	mu.Unlock()
	if _, err := g.refreshRules(context.Background(), &mihomo.Client{SettingsFile: cfg.settingsFile}); err != nil {
		t.Fatal(err)
	}
	check(true, false)
	a.Name = "Renamed A"
	activate(a)
	check(true, false)
	activate(b)
	check(false, true)
	// A failed application must retain B as the owner, including its choices.
	mu.Lock()
	failApply = true
	mu.Unlock()
	if _, err := g.activateProfileLocked(context.Background(), &state, a, true, nil); err == nil {
		t.Fatal("expected apply failure")
	}
	scope, err := g.currentRuleScope()
	if err != nil || scope != "profile:b" {
		t.Fatalf("failed switch changed owner: %q %v", scope, err)
	}
	check(false, true)
}

func TestLegacyRuleStateMigratesOnlyToItsCurrentSubscription(t *testing.T) {
	root := t.TempDir()
	cfg := config{configMetaFile: filepath.Join(root, "meta.json"), ruleStateFile: filepath.Join(root, "rule-state.json")}
	g := newGateway(cfg)
	_ = os.WriteFile(cfg.configMetaFile, []byte(`{"source":"profile","sourceId":"a"}`), 0600)
	_ = os.WriteFile(cfg.ruleStateFile, []byte(`{"version":1,"disabled":{"saved-rule":true}}`), 0600)
	a, err := g.loadRuleState()
	if err != nil || !a["saved-rule"] {
		t.Fatalf("migration failed: %v %v", a, err)
	}
	_ = os.WriteFile(cfg.configMetaFile, []byte(`{"source":"profile","sourceId":"b"}`), 0600)
	b, err := g.loadRuleState()
	if err != nil || len(b) != 0 {
		t.Fatalf("legacy choices leaked to B: %v %v", b, err)
	}
	if err := g.saveRuleState(map[string]bool{"b-rule": true}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(cfg.configMetaFile, []byte(`{"source":"profile","sourceId":"a"}`), 0600)
	a, err = g.loadRuleState()
	if err != nil || !a["saved-rule"] || a["b-rule"] {
		t.Fatalf("B save lost or changed A: %v %v", a, err)
	}
	g.config.profilesFile = filepath.Join(root, "profiles.json")
	_ = os.WriteFile(g.config.profilesFile, []byte(`{"current":"a","items":[]}`), 0600)
	_ = os.WriteFile(cfg.configMetaFile, []byte(`{"source":"nas-local","active":false}`), 0600)
	a, err = g.loadRuleState()
	if err != nil || !a["saved-rule"] {
		t.Fatalf("unapplied draft changed running subscription: %v %v", a, err)
	}
}
