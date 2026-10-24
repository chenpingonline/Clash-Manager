package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"time"
)

const appUpdateTokenHeader = "X-Clash-App-Update-Token"

type nativeConnectionKey struct{}

func nativeConnectionContext(ctx context.Context, conn net.Conn) context.Context {
	if conn.LocalAddr().Network() == "unix" {
		return context.WithValue(ctx, nativeConnectionKey{}, true)
	}
	return ctx
}

func (g *gateway) appUpdateCSRFToken() (string, error) {
	g.appUpdateTokenOnce.Do(func() {
		var token [32]byte
		if _, g.appUpdateTokenErr = rand.Read(token[:]); g.appUpdateTokenErr == nil {
			g.appUpdateToken = hex.EncodeToString(token[:])
		}
	})
	return g.appUpdateToken, g.appUpdateTokenErr
}

func (g *gateway) appUpdateRequestAllowed(r *http.Request) bool {
	return g.nativeMutationRequestAllowed(r, appUpdateTokenHeader)
}

// Only the native Unix-socket ingress may use a nonce when fnOS rewrites Host.
// Do not trust client-supplied forwarded headers on a TCP listener.
func (g *gateway) nativeMutationRequestAllowed(r *http.Request, tokenHeader string) bool {
	if sameOrigin(r) {
		return true
	}
	// fnOS authenticates before forwarding to our Unix socket. Its internal Host
	// need not match the browser Origin. A same-origin GET supplies this nonce;
	// third-party pages cannot read it or send this custom header without CORS.
	if (g.config.platform != "fnos" && g.config.platform != "") || g.config.listenAddr != "" || g.config.authPassword != "" || r.Context().Value(nativeConnectionKey{}) != true || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	token, err := g.appUpdateCSRFToken()
	return err == nil && subtle.ConstantTimeCompare([]byte(token), []byte(r.Header.Get(tokenHeader))) == 1
}

func (g *gateway) writeAppInstallStatus(w http.ResponseWriter, r *http.Request) {
	var result map[string]any
	if err := g.helperJSON(r.Context(), http.MethodGet, "/app/update/status", nil, &result, 10*time.Second); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	token, err := g.appUpdateCSRFToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成更新请求令牌失败"})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "更新状态响应无效"})
		return
	}
	result["csrfToken"] = token
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}
