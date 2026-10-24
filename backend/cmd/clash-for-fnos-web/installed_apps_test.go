package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstalledAppsUnavailableOnStandalone(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{"linux", "docker"} {
		g := newGateway(config{platform: platform})
		response := httptest.NewRecorder()
		if !g.handleSystemAPI(response, httptest.NewRequest(http.MethodGet, "/api/system/installed-apps", nil), "/api/system/installed-apps") || response.Code != http.StatusConflict {
			t.Fatalf("%s: %d %s", platform, response.Code, response.Body.String())
		}
	}
}

func TestInstalledAppsForwardsOnlyFixedReadOnlyHelperRoute(t *testing.T) {
	t.Parallel()
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/system/installed-apps" || r.URL.RawQuery != "" {
			t.Errorf("unexpected helper request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"music","name":"Music","rule":"PROCESS-PATH-REGEX,^/var/apps/music/target/,DIRECT"}]}`))
	}))
	g := newGateway(config{platform: "fnos", privilegedSocket: socket})
	response := httptest.NewRecorder()
	g.handleSystemAPI(response, httptest.NewRequest(http.MethodGet, "/api/system/installed-apps?root=/etc", nil), "/api/system/installed-apps")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"music"`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}
