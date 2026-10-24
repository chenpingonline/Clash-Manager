package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestBypassSetupTransaction(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "firewall-failure"}[failure], func(t *testing.T) {
			var mu sync.Mutex
			events := []string{}
			record := func(event string) { mu.Lock(); events = append(events, event); mu.Unlock() }
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut && r.URL.Path == "/configs" {
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					record("reload:" + body["payload"].(string))
					w.WriteHeader(204)
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/configs" {
					writeJSON(w, 200, map[string]any{"tun": map[string]bool{"enable": false}, "tproxy-port": 7896})
					return
				}
				if r.URL.Path == "/version" {
					writeJSON(w, 200, map[string]any{"version": "test"})
					return
				}
				http.NotFound(w, r)
			}))
			defer controller.Close()
			socket := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record(r.URL.Path)
				switch r.URL.Path {
				case "/config/active-raw":
					writeJSON(w, 200, map[string]any{"content": "mode: rule\n"})
				case "/network/bypass/prepare":
					writeJSON(w, 200, map[string]any{"txId": "bypass-tx", "effectiveContent": "mode: global\n", "settings": map[string]any{"tproxy": map[string]any{"port": 7896}}})
				case "/config/activate":
					writeJSON(w, 200, map[string]any{"method": "hot-reload"})
				case "/network/bypass/apply":
					if failure {
						writeJSON(w, 500, map[string]string{"error": "nft failed"})
					} else {
						writeJSON(w, 200, map[string]any{"ok": true})
					}
				case "/network/bypass/stop", "/config/rollback", "/config/commit":
					writeJSON(w, 200, map[string]any{"ok": true})
				default:
					http.NotFound(w, r)
				}
			}))
			g := newGateway(config{publicDir: t.TempDir(), gateway: "/app/clash-for-fnos", settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket})
			rec := httptest.NewRecorder()
			g.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/network/bypass/configure", strings.NewReader(`{"mode":"tproxy","interface":"eth0","address":"192.168.1.2","confirmed":true}`)))
			mu.Lock()
			got := strings.Join(events, "|")
			mu.Unlock()
			if failure {
				if rec.Code != 502 || !strings.Contains(got, "/network/bypass/stop|/config/rollback|reload:mode: rule") || strings.Contains(got, "/config/commit") {
					t.Fatalf("bad rollback: %d %s %s", rec.Code, got, rec.Body)
				}
			} else if rec.Code != 200 || !strings.Contains(got, "reload:mode: global\n|/network/bypass/apply|/config/commit") {
				t.Fatalf("bad activation: %d %s %s", rec.Code, got, rec.Body)
			}
		})
	}
}
func TestBypassSetupRequiresConfirmationAndSameOrigin(t *testing.T) {
	g := newGateway(config{publicDir: t.TempDir(), gateway: "/app/clash-for-fnos"})
	for _, path := range []string{"configure", "preview", "stop"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/network/bypass/"+path, strings.NewReader(`{}`))
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		g.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatal(path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/app/clash-for-fnos/api/network/bypass/configure", strings.NewReader(`{}`)))
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}
