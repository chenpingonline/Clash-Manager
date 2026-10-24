package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFirewallRuleReadForwarding(t *testing.T) {
	var calls atomic.Int32
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/network/bypass/rules" || r.URL.RawQuery != "" {
			t.Errorf("unexpected helper request: %s %s", r.Method, r.URL)
		}
		writeJSON(w, 200, map[string]any{"text": "table inet example {}", "truncated": false, "readAt": "2026-10-24T00:00:00Z"})
	}))
	g := newGateway(config{publicDir: t.TempDir(), gateway: "/app/clash-for-fnos", privilegedSocket: socket})
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/network/bypass/rules?command=flush", nil))
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), "table inet example") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/network/bypass/rules", strings.NewReader("{}")))
	if calls.Load() != 1 {
		t.Fatal("non-read request forwarded")
	}
}
