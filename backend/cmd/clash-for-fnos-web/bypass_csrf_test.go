package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeBypassThroughRewrittenHost(t *testing.T) {
	helper := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/network/bypass/preview" && r.URL.Path != "/network/bypass/stop" {
			t.Errorf("unexpected helper request %s", r.URL.Path)
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	g := newGateway(config{platform: "fnos", privilegedSocket: helper, gateway: "/app/clash-for-fnos"})
	dir, err := os.MkdirTemp("/tmp", "bypass-csrf-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "web.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: g, ConnContext: nativeConnectionContext}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	response, err := client.Get("http://internal/app/clash-for-fnos/api/network/bypass/csrf")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Token string `json:"csrfToken"`
	}
	err = json.NewDecoder(response.Body).Decode(&body)
	response.Body.Close()
	if err != nil || len(body.Token) != 64 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("invalid nonce response", err)
	}
	for _, action := range []string{"preview", "configure", "stop"} {
		for _, test := range []struct {
			name, token, site, origin string
			allowed                   bool
		}{
			{"rewritten host", body.Token, "same-origin", "https://nas.example:5667", true},
			{"no fetch metadata", body.Token, "", "https://nas.example:5667", true},
			{"same host without nonce", "", "same-origin", "http://internal", true},
			{"missing nonce", "", "same-origin", "https://nas.example:5667", false},
			{"wrong nonce", strings.Repeat("0", 64), "same-origin", "https://nas.example:5667", false},
			{"cross site", body.Token, "cross-site", "https://attacker.example", false},
			{"opaque origin", body.Token, "", "null", false},
			{"malformed origin", body.Token, "", "https://nas.example/path", false},
		} {
			t.Run(action+"/"+test.name, func(t *testing.T) {
				req, _ := http.NewRequest("POST", "http://internal/app/clash-for-fnos/api/network/bypass/"+action, strings.NewReader(`{}`))
				req.Header.Set(bypassTokenHeader, test.token)
				req.Header.Set("Origin", test.origin)
				req.Header.Set("Sec-Fetch-Site", test.site)
				response, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(response.Body)
				response.Body.Close()
				want := 403
				if test.allowed {
					want = 200
					if action == "configure" {
						want = 400
					}
				}
				if response.StatusCode != want {
					t.Fatalf("got %d want %d: %s", response.StatusCode, want, data)
				}
			})
		}
	}
	for _, cfg := range []config{{platform: "fnos"}, {platform: "fnos", listenAddr: ":8080"}, {platform: "fnos", authPassword: "password"}, {platform: "docker"}, {platform: "linux"}} {
		candidate := newGateway(cfg)
		token, _ := candidate.appUpdateCSRFToken()
		req := httptest.NewRequest("POST", "http://internal/api/network/bypass/preview", nil)
		req.Header.Set("Origin", "https://nas.example:5667")
		req.Header.Set(bypassTokenHeader, token)
		req.Header.Set("X-Forwarded-Host", "nas.example:5667")
		req.Header.Set("X-Trim-Userid", "1000")
		if candidate.nativeMutationRequestAllowed(req, bypassTokenHeader) {
			t.Fatal("TCP forwarded headers bypassed protection")
		}
		if cfg.platform != "fnos" || cfg.listenAddr != "" || cfg.authPassword != "" {
			req = req.WithContext(context.WithValue(req.Context(), nativeConnectionKey{}, true))
			if candidate.nativeMutationRequestAllowed(req, bypassTokenHeader) {
				t.Fatal("nonce fallback escaped native fnOS ingress")
			}
		}
	}
}
