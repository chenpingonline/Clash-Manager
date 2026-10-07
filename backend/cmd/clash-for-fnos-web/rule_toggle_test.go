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

func TestRuleToggleChecksLiveIdentityAndReadback(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		oldCore, changed, reject, ignore, enable bool
		status                                   int
	}{
		{name: "disable", status: 200}, {name: "enable", enable: true, status: 200},
		{name: "old core", oldCore: true, status: 409}, {name: "changed rules", changed: true, status: 409},
		{name: "rejected patch", reject: true, status: 500}, {name: "patch ignored", ignore: true, status: 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			disabled := tc.enable
			patches := 0
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rules":
					rule := map[string]any{"scope": "managed", "index": 7, "type": "DomainSuffix", "payload": "example.com", "proxy": "DIRECT", "extra": map[string]bool{"disabled": disabled}}
					if tc.oldCore {
						delete(rule, "index")
						delete(rule, "extra")
					}
					if tc.changed {
						rule["payload"] = "other.example"
					}
					writeJSON(w, 200, map[string]any{"rules": []any{rule}})
				case "/rules/disable":
					patches++
					if r.Method != http.MethodPatch {
						t.Errorf("method = %s", r.Method)
					}
					var patch map[string]bool
					_ = json.NewDecoder(r.Body).Decode(&patch)
					value, ok := patch["7"]
					if !ok || len(patch) != 1 || value == tc.enable {
						t.Errorf("wrong rule index/state: %v", patch)
					}
					if tc.reject {
						http.Error(w, "rejected", 500)
						return
					}
					if !tc.ignore {
						disabled = value
					}
					w.WriteHeader(204)
				default:
					http.NotFound(w, r)
				}
			}))
			defer controller.Close()
			root := t.TempDir()
			file := filepath.Join(root, "rules.json")
			// Stale cached rules must not authorize changing a different live rule.
			_ = os.WriteFile(file, []byte(`{"rules":[{"index":7,"type":"DomainSuffix","payload":"example.com","proxy":"DIRECT","extra":{"disabled":false}}]}`), 0600)
			g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), rulesSnapshotFile: file, ruleStateFile: filepath.Join(root, "rule-state.json")})
			body, _ := json.Marshal(map[string]any{"scope": "managed", "index": 7, "type": "DomainSuffix", "payload": "example.com", "proxy": "DIRECT", "disabled": !tc.enable})
			response := httptest.NewRecorder()
			g.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/rules/disable", strings.NewReader(string(body))))
			if response.Code != tc.status {
				t.Fatalf("toggle = %d %s, want %d", response.Code, response.Body.String(), tc.status)
			}
			if (tc.oldCore || tc.changed) && patches != 0 {
				t.Fatal("unsafe patch was sent")
			}
			if tc.status == 200 {
				stored, _ := os.ReadFile(file)
				if string(stored) != response.Body.String() || disabled == tc.enable {
					t.Fatal("confirmed state not returned and cached")
				}
			}
		})
	}
}

func TestRuleToggleRejectsMissingState(t *testing.T) {
	for _, body := range []string{`{}`, `{"index":-1,"disabled":true,"type":"Match","proxy":"DIRECT"}`, `{"scope":"managed","index":0,"type":"Match","proxy":"DIRECT"}`} {
		g := newGateway(config{})
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/rules/disable", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("invalid edit = %d %s", w.Code, w.Body.String())
		}
	}
}
