package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNetworkSettingsSummaryIsOptInAndForwarded(t *testing.T) {
	seen := make(chan string, 2)
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/network/status" {
			http.NotFound(w, r)
			return
		}
		seen <- r.URL.Query().Get("summary")
		writeJSON(w, 200, map[string]any{"settings": map[string]any{"mixed": map[string]any{"port": 7890, "enabled": true}}})
	}))
	g := newGateway(config{gateway: "/app/clash-for-fnos", privilegedSocket: socket, settingsFile: writeGatewaySettings(t, "http://127.0.0.1:1")})
	for _, query := range []string{"", "?summary=true"} {
		response := httptest.NewRecorder()
		g.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/network/settings"+query, nil))
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		expected := ""
		if query != "" {
			expected = "true"
		}
		if got := <-seen; got != expected {
			t.Fatalf("summary query %q, expected %q", got, expected)
		}
	}
}
