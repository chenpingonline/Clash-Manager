package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTunDiagnosticsAndFixedRepairAPI(t *testing.T) {
	var repairs atomic.Int32
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network/tun/diagnostics":
			if r.Method != http.MethodGet {
				t.Error("diagnostics changed state")
			}
			writeJSON(w, 200, map[string]any{"capability": map[string]any{"supported": false}, "repairable": true})
		case "/network/tun/repair":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 0 {
				t.Error("forwarded caller commands to repair", err)
			}
			repairs.Add(1)
			writeJSON(w, 200, map[string]any{"ok": true, "capability": map[string]any{"supported": true}})
		default:
			http.NotFound(w, r)
		}
	}))
	g := newGateway(config{gateway: "/app/clash-for-fnos", privilegedSocket: socket})
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/network/tun/diagnostics", nil))
	if rec.Code != 200 || repairs.Load() != 0 || !strings.Contains(rec.Body.String(), `"repairable":true`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/network/tun/repair", strings.NewReader(`{"command":"must-not-forward"}`)))
	if rec.Code != 200 || repairs.Load() != 1 {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
