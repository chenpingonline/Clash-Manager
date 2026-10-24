package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProxyAuthNetworkAPIPreservesCredentialFields(t *testing.T) {
	var forwarded atomic.Bool
	auth := map[string]any{"accounts": []any{map[string]any{"username": "test-user", "password": "test-only"}}, "skipAuthPrefixes": []any{"::1/128"}}
	socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/network/status":
			writeJSON(w, 200, map[string]any{"offline": true, "settings": map[string]any{"proxyAuthentication": auth}})
		case "/network/update":
			var input map[string]any
			if !decodeJSONBody(w, r, &input) {
				return
			}
			value, ok := input["proxyAuthentication"].(map[string]any)
			if !ok || len(value["accounts"].([]any)) != 1 {
				t.Error("proxy authentication was stripped by Gateway")
			}
			forwarded.Store(true)
			writeJSON(w, 200, map[string]any{"txId": "auth-tx", "settings": map[string]any{"proxyAuthentication": value}})
		case "/config/activate":
			writeJSON(w, 200, map[string]any{"method": "saved-only"})
		case "/config/commit", "/system/proxy-environment/sync":
			writeJSON(w, 200, map[string]any{})
		case "/status":
			writeJSON(w, 200, map[string]any{"mode": "managed"})
		default:
			http.NotFound(w, r)
		}
	}))
	g := newGateway(config{gateway: "/app/clash-for-fnos", privilegedSocket: socket, settingsFile: writeGatewaySettings(t, "http://127.0.0.1:1")})
	get := httptest.NewRecorder()
	g.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/network/settings", nil))
	if get.Code != 200 || get.Header().Get("Cache-Control") != "no-store" || !strings.Contains(get.Body.String(), `"proxyAuthentication"`) {
		t.Fatal("manual auth was not exposed as non-cacheable settings")
	}
	put := httptest.NewRecorder()
	g.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/app/clash-for-fnos/api/network/settings", strings.NewReader(`{"proxyAuthentication":{"accounts":[{"username":"test-user","password":"test-only"}],"skipAuthPrefixes":["::1/128"]}}`)))
	if !forwarded.Load() || put.Code != 200 || !strings.Contains(put.Body.String(), `"proxyAuthentication"`) {
		t.Fatalf("proxy auth update failed, status=%d", put.Code)
	}
}
