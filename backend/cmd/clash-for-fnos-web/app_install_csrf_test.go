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

func TestNativeUpdateThroughRewrittenHost(t *testing.T) {
	calls := 0
	helper := startTunHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/update/status" {
			writeJSON(w, 200, map[string]any{"supported": true, "status": map[string]any{"stage": "idle"}})
			return
		}
		if r.URL.Path != "/app/update/start" {
			t.Errorf("unexpected %s", r.URL)
			return
		}
		calls++
		writeJSON(w, 200, map[string]any{"supported": true, "status": map[string]any{"stage": "queued"}})
	}))
	g := newGateway(config{platform: "fnos", privilegedSocket: helper, gateway: "/app/clash-for-fnos"})
	dir, err := os.MkdirTemp("/tmp", "update-csrf-")
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
	response, err := client.Get("http://internal/app/clash-for-fnos/api/app/update/status")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	err = json.NewDecoder(response.Body).Decode(&body)
	response.Body.Close()
	if err != nil || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("invalid status response")
	}
	token, _ := body["csrfToken"].(string)
	if len(token) != 64 {
		t.Fatal("missing nonce")
	}
	for _, test := range []struct {
		name, token, site, origin string
		status                    int
	}{
		{"rewritten host", token, "same-origin", "https://nas.example:5667", 200},
		{"webview without metadata", token, "", "https://nas.example:5667", 200},
		{"missing nonce", "", "same-origin", "https://nas.example:5667", 403},
		{"wrong nonce", strings.Repeat("0", 64), "same-origin", "https://nas.example:5667", 403},
		{"cross site", token, "cross-site", "https://attacker.example", 403},
		{"invalid origin", token, "", "null", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "http://internal/app/clash-for-fnos/api/app/update", strings.NewReader(`{"version":"1.3.20"}`))
			req.Header.Set(appUpdateTokenHeader, test.token)
			req.Header.Set("Origin", test.origin)
			req.Header.Set("Sec-Fetch-Site", test.site)
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != test.status {
				t.Fatalf("got %d want %d", response.StatusCode, test.status)
			}
		})
	}
	if calls != 2 {
		t.Fatalf("unsafe request reached helper: %d", calls)
	}
	other := newGateway(config{platform: "fnos"})
	otherToken, _ := other.appUpdateCSRFToken()
	if token == otherToken {
		t.Fatal("nonce reused across processes")
	}
	for _, platform := range []string{"fnos", "docker", "linux"} {
		candidate := newGateway(config{platform: platform})
		candidate.appUpdateToken, candidate.appUpdateTokenErr = token, nil
		candidate.appUpdateTokenOnce.Do(func() {})
		req := httptest.NewRequest("POST", "http://internal/api/app/update", nil)
		req.Header.Set("Origin", "https://nas.example:5667")
		req.Header.Set(appUpdateTokenHeader, token)
		req.Header.Set("X-Forwarded-Host", "nas.example:5667")
		req.Header.Set("X-Trim-Userid", "1000")
		if candidate.appUpdateRequestAllowed(req) {
			t.Fatal("TCP header spoof accepted")
		}
	}
}
