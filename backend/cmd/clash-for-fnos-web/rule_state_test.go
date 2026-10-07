package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenpingonline/Clash-for-fnos/backend/internal/mihomo"
)

func TestRuleStateSurvivesRestartReorderAndRetry(t *testing.T) {
	rules := []map[string]any{
		{"index": 0, "type": "DomainSuffix", "payload": "example.com", "proxy": "DIRECT", "extra": map[string]bool{"disabled": false}},
		{"index": 1, "type": "Match", "payload": "", "proxy": "DIRECT", "extra": map[string]bool{"disabled": false}},
	}
	reject := false
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rules" {
			writeJSON(w, 200, map[string]any{"rules": rules})
			return
		}
		if r.URL.Path == "/rules/disable" {
			if reject {
				http.Error(w, "temporarily unavailable", 503)
				return
			}
			var patch map[int]bool
			_ = json.NewDecoder(r.Body).Decode(&patch)
			for _, rule := range rules {
				if value, ok := patch[rule["index"].(int)]; ok {
					rule["extra"].(map[string]bool)["disabled"] = value
				}
			}
			w.WriteHeader(204)
			return
		}
		http.NotFound(w, r)
	}))
	defer controller.Close()
	cfg := config{settingsFile: writeGatewaySettings(t, controller.URL), ruleStateFile: filepath.Join(t.TempDir(), "rule-state.json")}
	toggle := func(g *gateway, index int, disabled bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"scope": "managed", "index": index, "type": "DomainSuffix", "payload": "example.com", "proxy": "DIRECT", "disabled": disabled})
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/rules/disable", strings.NewReader(string(body))))
		if w.Code != 200 {
			t.Fatalf("toggle failed: %d %s", w.Code, w.Body.String())
		}
	}
	g := newGateway(cfg)
	toggle(g, 0, true)
	// Both the web process and Core restart. A subscription inserted another
	// rule at index 0, moved our rule to index 2 and changed the final rule.
	rules[0]["index"] = 2
	rules[0]["extra"] = map[string]bool{"disabled": false}
	rules[1]["index"] = 3
	rules = append([]map[string]any{{"index": 0, "type": "DomainSuffix", "payload": "other.com", "proxy": "DIRECT", "extra": map[string]bool{"disabled": false}}}, rules...)
	g = newGateway(cfg)
	client := &mihomo.Client{SettingsFile: cfg.settingsFile}
	reject = true
	if _, err := g.refreshRules(context.Background(), client); err == nil {
		t.Fatal("restore failure not reported")
	}
	reject = false
	if _, err := g.refreshRules(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if rules[0]["extra"].(map[string]bool)["disabled"] || !rules[1]["extra"].(map[string]bool)["disabled"] || rules[2]["extra"].(map[string]bool)["disabled"] {
		t.Fatal("saved choice did not follow content after restart/reorder/retry")
	}
	// A changed target policy must not inherit the old choice.
	rules[1]["proxy"] = "REJECT"
	rules[1]["extra"] = map[string]bool{"disabled": false}
	if _, err := g.refreshRules(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if rules[1]["extra"].(map[string]bool)["disabled"] {
		t.Fatal("changed rule inherited saved choice")
	}
	rules[1]["proxy"] = "DIRECT"
	toggle(g, 2, false)
	g = newGateway(cfg)
	if _, err := g.refreshRules(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	state, err := g.loadRuleState()
	if err != nil || len(state) != 0 || rules[1]["extra"].(map[string]bool)["disabled"] {
		t.Fatal("reenabled rule was disabled again")
	}
}

func TestRuleStateDuplicateChangesDoNotTransferChoice(t *testing.T) {
	before := []byte(`{"rules":[{"index":0,"type":"Domain","payload":"x","proxy":"DIRECT","extra":{"disabled":false}},{"index":1,"type":"Domain","payload":"x","proxy":"DIRECT","extra":{"disabled":false}}]}`)
	after := []byte(`{"rules":[{"index":0,"type":"Domain","payload":"x","proxy":"DIRECT","extra":{"disabled":false}}]}`)
	oldKeys, _, _ := ruleStateKeys(before)
	newKeys, _, _ := ruleStateKeys(after)
	if oldKeys[0] == oldKeys[1] || oldKeys[0] == newKeys[0] || oldKeys[1] == newKeys[0] {
		t.Fatal("ambiguous duplicates share a saved identity")
	}
}

func TestRuleToggleSaveFailureRollsBackRuntime(t *testing.T) {
	disabled := false
	parent := filepath.Join(t.TempDir(), "blocked")
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rules" {
			writeJSON(w, 200, map[string]any{"rules": []any{map[string]any{"scope": "managed", "index": 0, "type": "Match", "payload": "", "proxy": "DIRECT", "extra": map[string]bool{"disabled": disabled}}}})
			return
		}
		var patch map[int]bool
		_ = json.NewDecoder(r.Body).Decode(&patch)
		disabled = patch[0]
		if disabled {
			_ = os.WriteFile(parent, []byte("block directory creation"), 0600)
		}
		w.WriteHeader(204)
	}))
	defer controller.Close()
	// Storage becomes unwritable after reading the previous state but before saving.
	g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), ruleStateFile: filepath.Join(parent, "state.json")})
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/rules/disable", strings.NewReader(`{"scope":"managed","index":0,"type":"Match","payload":"","proxy":"DIRECT","disabled":true}`)))
	if w.Code != 500 || disabled || !strings.Contains(w.Body.String(), "保存规则开关失败") {
		t.Fatalf("save failure did not roll back: %d %s disabled=%v", w.Code, w.Body.String(), disabled)
	}
	// Corrupt state must remain intact, never be silently overwritten.
	g.config.ruleStateFile = filepath.Join(t.TempDir(), "state.json")
	_ = os.WriteFile(g.config.ruleStateFile, []byte("broken"), 0600)
	w = httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/rules/disable", strings.NewReader(`{"scope":"managed","index":0,"type":"Match","payload":"","proxy":"DIRECT","disabled":true}`)))
	stored, _ := os.ReadFile(g.config.ruleStateFile)
	if w.Code != 500 || disabled || string(stored) != "broken" {
		t.Fatal("corrupt saved state was overwritten")
	}
}
