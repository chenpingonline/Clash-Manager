package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerAuthentication(t *testing.T) {
	g := newGateway(config{platform: "docker", authUser: "admin", authPassword: "docker-test-password"})
	request := func(method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, req)
		return rec
	}
	if rec := request("GET", "/api/runtime", "", nil, ""); rec.Code != 401 {
		t.Fatalf("unauthenticated runtime: %d", rec.Code)
	}
	if rec := request("GET", "/api/health", "", nil, ""); rec.Code != 200 {
		t.Fatalf("health: %d", rec.Code)
	}
	if rec := request("POST", "/api/auth/login", `{"username":"admin","password":"wrong"}`, nil, ""); rec.Code != 401 {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	login := request("POST", "/api/auth/login", `{"username":"admin","password":"docker-test-password"}`, nil, "http://localhost")
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("missing protected session cookie")
	}
	cookie := cookies[0]
	if rec := request("GET", "/api/runtime", "", cookie, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"platform":"docker"`) {
		t.Fatalf("runtime: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request("PUT", "/api/system/proxy-environment", `{}`, cookie, "http://localhost"); rec.Code != 409 {
		t.Fatalf("native feature should be blocked: %d", rec.Code)
	}
	if rec := request("PUT", "/api/settings", `{}`, cookie, "https://attacker.example"); rec.Code != 403 {
		t.Fatalf("CSRF accepted: %d", rec.Code)
	}
	cookie.Value += "x"
	if rec := request("GET", "/api/runtime", "", cookie, ""); rec.Code != 401 {
		t.Fatalf("tampered token accepted: %d", rec.Code)
	}
	cookie.Value = g.sessionToken(time.Now().Add(-time.Hour))
	if rec := request("GET", "/api/runtime", "", cookie, ""); rec.Code != 401 {
		t.Fatalf("expired token accepted: %d", rec.Code)
	}
}

func TestHTTPRequiresPasswordAndSupportsSecretFile(t *testing.T) {
	t.Setenv("APP_AUTH_PASSWORD_FILE", "")
	cfg := config{platform: "docker"}
	if configureAuth(&cfg) == nil {
		t.Fatal("Docker accepted missing password")
	}
	cfg = config{platform: "fnos", listenAddr: ":8080"}
	if configureAuth(&cfg) == nil {
		t.Fatal("HTTP accepted missing password")
	}
	secret := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(secret, []byte("docker-secret-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_AUTH_PASSWORD_FILE", secret)
	cfg = config{platform: "docker"}
	if err := configureAuth(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.authPassword != "docker-secret-password" || cfg.listenAddr != ":8080" {
		t.Fatal("secret file or default listener failed")
	}
}

func TestHTTPPasswordMinimumLength(t *testing.T) {
	t.Setenv("APP_AUTH_PASSWORD_FILE", "")
	for _, deployment := range []config{{platform: "docker"}, {platform: "fnos", listenAddr: ":8080"}} {
		for _, password := range []string{"", "1234567", "12345678", "longer-test-password"} {
			cfg := deployment
			cfg.authPassword = password
			err := configureAuth(&cfg)
			if (err == nil) != (len(password) >= 8) {
				t.Fatalf("platform=%s length=%d: %v", cfg.platform, len(password), err)
			}
			if err != nil && !strings.Contains(err.Error(), "至少 8 字符") {
				t.Fatalf("outdated password validation message: %v", err)
			}
		}
	}
	// Native Unix-socket deployment still uses fnOS authentication.
	if err := configureAuth(&config{platform: "fnos"}); err != nil {
		t.Fatalf("native Unix-socket deployment changed: %v", err)
	}
	secret := filepath.Join(t.TempDir(), "password")
	t.Setenv("APP_AUTH_PASSWORD_FILE", secret)
	for _, password := range []string{"1234567", "12345678"} {
		if err := os.WriteFile(secret, []byte(password+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg := config{platform: "docker"}
		if err := configureAuth(&cfg); (err == nil) != (len(password) >= 8) {
			t.Fatalf("password file length=%d: %v", len(password), err)
		}
	}
}

func TestNativeGatewayAndDockerRoot(t *testing.T) {
	t.Setenv("APP_PLATFORM", "fnos")
	t.Setenv("GATEWAY_PREFIX", "/app/clash-for-fnos")
	if gatewayPrefix() != "/app/clash-for-fnos" {
		t.Fatal("native prefix changed")
	}
	t.Setenv("APP_PLATFORM", "docker")
	t.Setenv("GATEWAY_PREFIX", "")
	cfg := loadConfig()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "index.html"), []byte("docker-root"), 0600)
	cfg.publicDir = root
	g := newGateway(cfg)
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "docker-root") {
		t.Fatalf("root access: %d %s", rec.Code, rec.Body.String())
	}
	native := newGateway(config{})
	rec = httptest.NewRecorder()
	native.ServeHTTP(rec, httptest.NewRequest("GET", "/api/runtime", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"appIcons":true`) {
		t.Fatalf("native runtime changed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimit(t *testing.T) {
	limiter := loginLimiter{}
	for i := 0; i < 10; i++ {
		if !limiter.allow("127.0.0.1") {
			t.Fatal("limited too soon")
		}
	}
	if limiter.allow("127.0.0.1") {
		t.Fatal("unlimited login attempts")
	}
	if !limiter.allow("127.0.0.2") {
		t.Fatal("blocked unrelated client")
	}
}
