package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedTunEnableRestartsBeforeVerifyingRealInterface(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(map[bool]string{true: "ready", false: "missing-interface"}[ready], func(t *testing.T) {
			var running, rolledBack atomic.Bool
			var patches, commits, restarts atomic.Int32
			var mu sync.Mutex
			events := []string{}
			socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				events = append(events, r.URL.Path)
				mu.Unlock()
				switch r.URL.Path {
				case "/network/tun":
					writeJSON(w, 200, map[string]any{"txId": "modern", "previousEnabled": false, "managed": true, "offline": false})
				case "/config/validate", "/config/activate":
					writeJSON(w, 200, map[string]any{"ok": true})
				case "/config/commit":
					commits.Add(1)
					writeJSON(w, 200, map[string]any{})
				case "/config/rollback":
					rolledBack.Store(true)
					writeJSON(w, 200, map[string]any{})
				case "/core/restart-managed":
					restarts.Add(1)
					running.Store(!rolledBack.Load())
					writeJSON(w, 200, map[string]any{})
				case "/network/tun/interface":
					writeJSON(w, 200, tunInterfaceState{Name: "Meta", Checked: true, Exists: running.Load() && ready, Up: running.Load() && ready, TUN: running.Load() && ready})
				case "/status":
					writeJSON(w, 200, map[string]any{"mode": "managed", "canRestartService": true})
				default:
					http.NotFound(w, r)
				}
			}))
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					patches.Add(1)
					w.WriteHeader(204)
					return
				}
				if r.URL.Path == "/version" {
					writeJSON(w, 200, map[string]any{"version": "test"})
					return
				}
				writeJSON(w, 200, map[string]any{"tun": map[string]any{"enable": running.Load(), "device": "Meta"}})
			}))
			defer controller.Close()
			g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket})
			rec := httptest.NewRecorder()
			g.updateTun(rec, httptest.NewRequest(http.MethodPut, "/api/network/tun", strings.NewReader(`{"enabled":true}`)))
			if patches.Load() != 0 {
				t.Fatal("managed enable used PATCH", patches.Load())
			}
			mu.Lock()
			sequence := strings.Join(events, ",")
			mu.Unlock()
			if !strings.HasPrefix(sequence, "/network/tun,/config/validate,/config/activate,/core/restart-managed") {
				t.Fatal(sequence)
			}
			if ready {
				if rec.Code != 200 || commits.Load() != 1 || restarts.Load() != 1 {
					t.Fatal(rec.Code, rec.Body.String())
				}
			} else {
				if rec.Code != 502 || commits.Load() != 0 || !rolledBack.Load() || running.Load() {
					t.Fatal("missing interface was reported as enabled", rec.Code, rec.Body.String())
				}
			}
		})
	}
}
func TestUnifiedTunStateDistinguishesIntentRuntimeAndInterface(t *testing.T) {
	var ready, unknown, offline atomic.Bool
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/network/status" {
			writeJSON(w, 200, map[string]any{"settings": map[string]any{"tun": map[string]any{"enabled": true}}, "offline": offline.Load()})
			return
		}
		writeJSON(w, 200, tunInterfaceState{Name: "Meta", Checked: !unknown.Load(), Exists: ready.Load(), TUN: ready.Load(), Up: ready.Load()})
	}))
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"tun": map[string]any{"enable": true, "device": "Meta"}})
	}))
	defer controller.Close()
	g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket})
	if s := g.readTunState(context.Background()); s.Phase != "mismatch" || s.DesiredEnabled == nil || !*s.DesiredEnabled || s.RuntimeEnabled == nil || !*s.RuntimeEnabled {
		t.Fatal(s)
	}
	unknown.Store(true)
	if s := g.readTunState(context.Background()); s.Phase != "unknown" {
		t.Fatal(s)
	}
	unknown.Store(false)
	ready.Store(true)
	if s := g.readTunState(context.Background()); s.Phase != "enabled" {
		t.Fatal(s)
	}
	g.setTunOperation(true, false, "restart-managed")
	if s := g.readTunState(context.Background()); s.Phase != "switching" || s.RuntimeEnabled != nil {
		t.Fatal(s)
	}
	g.setTunOperation(false, false, "done")
	offline.Store(true)
	if s := g.readTunState(context.Background()); s.Phase != "stopped" || s.DesiredEnabled == nil || !*s.DesiredEnabled {
		t.Fatal("stopped Core erased intent", s)
	}
}
func TestInterfaceVerificationDoesNotAcceptControllerFlagAlone(t *testing.T) {
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, tunInterfaceState{Name: "Meta", Checked: true})
	}))
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"tun": map[string]any{"enable": true, "device": "Meta"}})
	}))
	defer controller.Close()
	g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket})
	if err := g.waitVerifiedTun(context.Background(), true, 100*time.Millisecond); err == nil {
		t.Fatal("accepted config flag without a kernel interface")
	}
}

func TestStartupHonorsDeliberatelyStoppedCore(t *testing.T) {
	var controllerRequests atomic.Int32
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { controllerRequests.Add(1); http.NotFound(w, r) }))
	defer controller.Close()
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Error("startup touched a deliberately stopped Core", r.URL.Path)
		}
		writeJSON(w, 200, map[string]any{"mode": "managed", "coreRunIntent": false})
	}))
	g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket})
	g.reconcileManagedTunAfterStartup(context.Background())
	if controllerRequests.Load() != 0 {
		t.Fatal("waited for or restarted an intentionally stopped Core")
	}
}
