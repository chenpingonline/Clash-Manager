package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBypassMonitorOnlyForwardsReads(t *testing.T) {
	var reads atomic.Int32
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/network/bypass" {
			t.Errorf("unexpected mutation or route: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		reads.Add(1)
		writeJSON(w, 200, map[string]any{"interfaces": []any{}, "forwarding": nil, "firewall": map[string]bool{"readable": false}})
	}))
	g := newGateway(config{publicDir: t.TempDir(), gateway: "/app/clash-for-fnos", privilegedSocket: socket})
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/network/bypass", nil))
	if rec.Code != 200 || reads.Load() != 1 || !strings.Contains(rec.Body.String(), `"forwarding":null`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/app/clash-for-fnos/api/network/bypass", strings.NewReader(`{"enabled":true}`)))
	if reads.Load() != 1 || rec.Code < 400 {
		t.Fatalf("write API still enabled: %d %s", rec.Code, rec.Body.String())
	}
}
