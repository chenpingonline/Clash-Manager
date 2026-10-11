package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppUpdateInfoReturnsDetectedReleaseWithoutNetwork(t *testing.T) {
	g := newGateway(config{settingsFile: "", releaseRepo: "invalid-no-network"})
	g.appUpdateCache = map[string]any{"currentVersion": "1.3.15", "updateAvailable": true, "latest": map[string]any{"tag": "v1.3.16"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := g.appUpdateStatus(ctx, false)
	if err != nil || result["updateAvailable"] != true || result["latest"].(map[string]any)["tag"] != "v1.3.16" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestAppUpdateRoutesAndCSRF(t *testing.T) {
	t.Parallel()
	calls := 0
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == http.MethodGet && r.URL.Path == "/app/update/status" {
			writeJSON(w, 200, map[string]any{"supported": true, "status": map[string]any{"stage": "idle"}})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/app/update/start" {
			var body map[string]any
			if !decodeJSONBody(w, r, &body) {
				return
			}
			if body["version"] != "v1.3.16" {
				t.Error(body)
			}
			writeJSON(w, 200, map[string]any{"status": map[string]any{"stage": "queued"}})
			return
		}
		t.Errorf("unexpected helper route %s %s", r.Method, r.URL.Path)
	}))
	g := newGateway(config{platform: "fnos", privilegedSocket: socket})
	for _, request := range []*http.Request{httptest.NewRequest("GET", "/api/app/update/status", nil), httptest.NewRequest("POST", "/api/app/update", strings.NewReader(`{"version":"v1.3.16"}`))} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, request)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	request := httptest.NewRequest("POST", "/api/app/update", strings.NewReader(`{"version":"v1.3.16"}`))
	request.Header.Set("Origin", "https://untrusted.test")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, request)
	if w.Code != 403 || calls != 2 {
		t.Fatalf("CSRF accepted %d calls=%d", w.Code, calls)
	}
}

func TestStandaloneRejectsNativeAppInstaller(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{"docker", "linux"} {
		g := newGateway(config{platform: platform, authPassword: "test-password"})
		for _, route := range []string{"/api/app/update", "/api/app/update/status"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", route, strings.NewReader(`{}`))
			if !g.handleUnsupportedStandalone(w, r, route) || w.Code != 409 {
				t.Fatalf("%s %s: %d", platform, route, w.Code)
			}
		}
	}
}
