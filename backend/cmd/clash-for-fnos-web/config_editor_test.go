package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigEditorTransaction(t *testing.T) {
	for _, tc := range []struct {
		name                                             string
		mode                                             string
		stale, invalid, failApply, switched, changedPath bool
		status                                           int
	}{
		{name: "save persists startup and manager config", mode: "managed", status: 200},
		{name: "external core stays read only", mode: "external", status: 403},
		{name: "stale edit rejected", mode: "managed", stale: true, status: 409},
		{name: "startup path changed", mode: "managed", changedPath: true, status: 409},
		{name: "switched to external after opening", mode: "managed", switched: true, status: 403},
		{name: "invalid yaml keeps old config", mode: "managed", invalid: true, status: 502},
		{name: "reload failure rolls back", mode: "managed", failApply: true, status: 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			original := "mixed-port: 7890\nrules: [\"MATCH,DIRECT\"]\n"
			disk, runtime, candidate := original, original, ""
			activeMode, activePath := tc.mode, "/test/startup.yaml"
			prepares, commits, rollbacks := 0, 0, 0
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/version":
					writeJSON(w, 200, map[string]string{"version": "test"})
				case "/configs":
					var body struct{ Payload string }
					_ = json.NewDecoder(r.Body).Decode(&body)
					if tc.failApply && strings.Contains(body.Payload, "7891") {
						http.Error(w, "injected reload failure", 500)
						return
					}
					runtime = body.Payload
					w.WriteHeader(204)
				default:
					http.NotFound(w, r)
				}
			}))
			defer controller.Close()
			root := t.TempDir()
			managed := filepath.Join(root, "managed.yaml")
			if err := os.WriteFile(managed, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/config/active-raw":
					writeJSON(w, 200, map[string]any{"mode": activeMode, "path": activePath, "content": disk})
				case "/config/sync":
					prepares++
					var body struct{ Content string }
					_ = json.NewDecoder(r.Body).Decode(&body)
					var parsed map[string]any
					if err := yaml.Unmarshal([]byte(body.Content), &parsed); err != nil {
						http.Error(w, "invalid yaml", 400)
						return
					}
					candidate = body.Content
					writeJSON(w, 200, map[string]any{"txId": "editor-test", "target": "/test/startup.yaml", "effectiveContent": candidate})
				case "/config/activate":
					disk = candidate
					writeJSON(w, 200, map[string]string{"method": "hot-reload"})
				case "/config/rollback":
					rollbacks++
					disk = original
					writeJSON(w, 200, map[string]bool{"ok": true})
				case "/config/commit":
					commits++
					writeJSON(w, 200, map[string]bool{"ok": true})
				default:
					http.NotFound(w, r)
				}
			}))
			g := newGateway(config{privilegedSocket: socket, settingsFile: writeGatewaySettings(t, controller.URL), managedConfigFile: managed, configMetaFile: filepath.Join(root, "meta.json"), backupDir: filepath.Join(root, "backups")})
			get := httptest.NewRecorder()
			g.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/config/editor", nil))
			if tc.mode == "external" && get.Code != 403 {
				t.Fatalf("external GET = %d", get.Code)
			}
			var opened struct{ Content, Revision string }
			_ = json.Unmarshal(get.Body.Bytes(), &opened)
			if tc.mode == "managed" && (get.Code != 200 || opened.Content != original || opened.Revision == "") {
				t.Fatalf("editor GET = %d %s", get.Code, get.Body.String())
			}
			if tc.stale {
				mu.Lock()
				disk = "mixed-port: 7892\n"
				mu.Unlock()
			}
			mu.Lock()
			if tc.switched {
				activeMode = "external"
			}
			if tc.changedPath {
				activePath = "/test/other.yaml"
			}
			mu.Unlock()
			content := "mixed-port: 7891\nrules: [\"MATCH,DIRECT\"]\n"
			if tc.invalid {
				content = "rules: ["
			}
			revision := opened.Revision
			if revision == "" {
				revision = "external"
			}
			body, _ := json.Marshal(map[string]string{"content": content, "revision": revision})
			saved := httptest.NewRecorder()
			g.ServeHTTP(saved, httptest.NewRequest(http.MethodPut, "/api/config/editor", strings.NewReader(string(body))))
			if saved.Code != tc.status {
				t.Fatalf("save = %d %s, want %d", saved.Code, saved.Body.String(), tc.status)
			}
			mu.Lock()
			defer mu.Unlock()
			stored, err := os.ReadFile(managed)
			if err != nil {
				t.Fatal(err)
			}
			if tc.status == 200 {
				if !strings.Contains(disk, "7891") || runtime != disk || string(stored) != disk || commits != 1 {
					t.Fatalf("save did not persist consistently")
				}
				entries, err := os.ReadDir(filepath.Join(root, "backups"))
				if err != nil || len(entries) != 1 {
					t.Fatalf("backup missing: %v", err)
				}
			} else {
				if string(stored) != original || runtime != original || commits != 0 {
					t.Fatal("failed edit changed saved/runtime config")
				}
				if tc.failApply && (disk != original || rollbacks != 1) {
					t.Fatal("reload failure did not roll back startup config")
				}
				if (tc.stale || tc.mode == "external" || tc.switched || tc.changedPath) && prepares != 0 {
					t.Fatal("rejected edit entered transaction")
				}
			}
		})
	}
}
