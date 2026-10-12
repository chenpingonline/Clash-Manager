package main

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExitLocationUsesManualProxyCredentials(t *testing.T) {
	for _, test := range []struct {
		name        string
		proxyStatus int
		wantStatus  int
	}{
		{"authenticated", http.StatusOK, http.StatusOK},
		{"rejected", http.StatusProxyAuthRequired, http.StatusBadGateway},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotAuth := make(chan string, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth <- r.Header.Get("Proxy-Authorization")
				w.WriteHeader(test.proxyStatus)
				_, _ = w.Write([]byte(`{"success":true,"ip":"203.0.113.42","country_code":"hk"}`))
			}))
			defer proxy.Close()
			_, portText, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
			port, _ := strconv.Atoi(portText)
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/configs" {
					writeJSON(w, 200, map[string]any{"mixed-port": port})
					return
				}
				http.NotFound(w, r)
			}))
			defer controller.Close()
			socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/config/active-raw" {
					http.NotFound(w, r)
					return
				}
				writeJSON(w, 200, map[string]any{"content": "authentication: ['test-user:test-p:a@ss']\nskip-auth-prefixes: []\n"})
			}))
			g := newGateway(config{gateway: "/app/clash-for-fnos", settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket, exitLocationURL: "http://exit.invalid/json"})
			recorder := httptest.NewRecorder()
			g.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/exit-location", nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d", recorder.Code)
			}
			wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("test-user:test-p:a@ss"))
			if <-gotAuth != wantAuth {
				t.Fatal("manual credentials were not sent to the proxy")
			}
			body := recorder.Body.String()
			if strings.Contains(body, "test-user") || strings.Contains(body, "test-p:") || strings.Contains(body, wantAuth) {
				t.Fatal("response leaked proxy credentials")
			}
			if test.proxyStatus == http.StatusOK && !strings.Contains(body, `"via":"mixed"`) {
				t.Fatal("exit location did not use the proxy")
			}
			if test.proxyStatus == http.StatusProxyAuthRequired && !strings.Contains(body, "代理认证失败") {
				t.Fatal("missing useful authentication error")
			}
		})
	}
}

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
