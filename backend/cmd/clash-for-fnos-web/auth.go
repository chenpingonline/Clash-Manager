package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "clash_manager_session"
const sessionLifetime = 12 * time.Hour

type loginAttempt struct {
	count int
	reset time.Time
}
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.attempts == nil {
		l.attempts = make(map[string]loginAttempt)
	}
	for key, item := range l.attempts {
		if now.After(item.reset) {
			delete(l.attempts, key)
		}
	}
	item := l.attempts[ip]
	if item.count == 0 {
		item.reset = now.Add(time.Minute)
	}
	if item.count >= 10 || len(l.attempts) >= 4096 && item.count == 0 {
		return false
	}
	item.count++
	l.attempts[ip] = item
	return true
}

func (g *gateway) sessionToken(expiry time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%s", expiry.Unix(), g.config.authUser)))
	key := sha256.Sum256([]byte(g.config.authPassword))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (g *gateway) authenticated(r *http.Request) bool {
	if g.config.authPassword == "" {
		return (g.config.platform == "fnos" || g.config.platform == "") && g.config.listenAddr == ""
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	fields := strings.SplitN(string(raw), ":", 2)
	if len(fields) != 2 || fields[1] != g.config.authUser {
		return false
	}
	expiry, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || expiry <= time.Now().Unix() || expiry > time.Now().Add(sessionLifetime).Unix() {
		return false
	}
	expected := g.sessionToken(time.Unix(expiry, 0))
	return hmac.Equal([]byte(cookie.Value), []byte(expected))
}
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // CLI requests carry no browser credentials automatically.
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host == r.Host
}
func (g *gateway) authorize(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(stripPrefix(r.URL.Path, g.config.gateway), "/api/") {
		return true
	}
	if !g.authenticated(r) {
		writeJSON(w, 401, map[string]string{"error": "请先登录管理界面"})
		return false
	}
	if g.config.authPassword != "" && r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
		writeJSON(w, 403, map[string]string{"error": "拒绝跨站请求"})
		return false
	}
	return true
}
func (g *gateway) handleAuth(w http.ResponseWriter, r *http.Request, path string) bool {
	if !strings.HasPrefix(path, "/api/auth/") {
		return false
	}
	switch {
	case path == "/api/auth/session" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]bool{"required": g.config.authPassword != "" || (g.config.platform == "docker" || g.config.platform == "linux"), "authenticated": g.authenticated(r)})
	case path == "/api/auth/login" && r.Method == http.MethodPost:
		if !sameOrigin(r) {
			writeJSON(w, 403, map[string]string{"error": "拒绝跨站请求"})
			return true
		}
		if g.config.authPassword == "" {
			writeJSON(w, 409, map[string]string{"error": "当前入口未启用独立登录"})
			return true
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !g.loginLimiter.allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, 429, map[string]string{"error": "尝试次数过多，请稍后重试"})
			return true
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
			writeJSON(w, 400, map[string]string{"error": "登录请求无效"})
			return true
		}
		actual := sha256.Sum256([]byte(body.Username + "\x00" + body.Password))
		expected := sha256.Sum256([]byte(g.config.authUser + "\x00" + g.config.authPassword))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			writeJSON(w, 401, map[string]string{"error": "用户名或密码错误"})
			return true
		}
		expiry := time.Now().Add(sessionLifetime)
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: g.sessionToken(expiry), Path: "/", HttpOnly: true, Secure: g.config.authSecure || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionLifetime.Seconds()), Expires: expiry})
		writeJSON(w, 200, map[string]bool{"ok": true})
	case path == "/api/auth/logout" && r.Method == http.MethodPost:
		if !sameOrigin(r) {
			writeJSON(w, 403, map[string]string{"error": "拒绝跨站请求"})
			return true
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: g.config.authSecure || r.TLS != nil, SameSite: http.SameSiteStrictMode})
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeJSON(w, 404, map[string]string{"error": "Not found"})
	}
	return true
}
