package main

import (
	"context"
	"encoding/json"
	"github.com/chenpingonline/Clash-Manager/backend/internal/logstore"
	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomolog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogSettingsHistoryAndSeparateCoreClear(t *testing.T) {
	dir := t.TempDir()
	socketDir, e := os.MkdirTemp("/tmp", "log-test-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(socketDir)
	socket := filepath.Join(socketDir, "helper.sock")
	listener, e := net.Listen("unix", socket)
	if e != nil {
		t.Fatal(e)
	}
	var cleared atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/logs/core" {
			if r.URL.Query().Get("file") != "" {
				t.Error("forwarded arbitrary file")
			}
			if r.URL.Query().Get("cursor") == "expired" {
				writeJSON(w, 409, map[string]string{"error": logstore.ErrCursorExpired.Error()})
				return
			}
			json.NewEncoder(w).Encode(logstore.RawPage{Lines: []logstore.RawLine{{Key: "1:0", Message: r.URL.Query().Get("search")}}})
			return
		}
		if r.URL.Path == "/logs/clear" {
			cleared.Add(1)
		}
		json.NewEncoder(w).Encode(map[string]any{"available": true, "size": 123, "maxBytes": 10485760})
	})}
	go server.Serve(listener)
	defer server.Close()
	cfg := config{settingsFile: filepath.Join(dir, "settings.json"), mihomoLogFile: filepath.Join(dir, "mihomo.log"), privilegedSocket: socket}
	g := newGateway(cfg)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	settings := logstore.Defaults()
	settings.History = logstore.Policy{Days: 3, MaxMiB: 2}
	settings.Core = logstore.Policy{Days: 0, MaxMiB: 1}
	settings.SaveLevel = "debug"
	body, _ := json.Marshal(settings)
	w := request("PUT", "/api/logs/settings", string(body))
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	reloaded := newGateway(cfg)
	g = reloaded
	now := time.Now().UTC()
	for _, m := range []string{"a", "b", "c"} {
		if e = g.logs.Append(mihomolog.Record{Time: now.Format(time.RFC3339Nano), Level: "debug", Message: m}); e != nil {
			t.Fatal(e)
		}
	}
	page, e := g.logs.Query(context.Background(), logstore.Query{Level: "debug", Limit: 1})
	if e != nil || !page.HasMore {
		t.Fatalf("page %+v %v", page, e)
	}
	w = request("GET", "/api/logs/history?level=debug&limit=1&search=c&from="+now.Add(-time.Hour).Format(time.RFC3339)+"&to="+now.Add(time.Hour).Format(time.RFC3339), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"message":"c"`) {
		t.Fatalf("query %d %s", w.Code, w.Body.String())
	}
	if w = request("PUT", "/api/logs/settings", `{"history":{"days":-1,"maxMiB":2}}`); w.Code != 400 {
		t.Fatal("invalid accepted")
	}
	saved, _ := logstore.LoadSettings(g.logSettingsFile())
	if saved != settings {
		t.Fatal("invalid request overwrote settings")
	}
	if w = request("DELETE", "/api/logs/core", ""); w.Code != 200 || cleared.Load() != 1 {
		t.Fatalf("core clear %d", w.Code)
	}
	if w = request("GET", "/api/logs/core?search=panic%20%26%20startup&file=%2Fetc%2Fpasswd&limit=100", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `panic \u0026 startup`) {
		t.Fatalf("raw forward %d %s", w.Code, w.Body.String())
	}
	if w = request("GET", "/api/logs/core?limit=2001", ""); w.Code != 400 {
		t.Fatal("raw oversized limit accepted")
	}
	if w = request("GET", "/api/logs/core?cursor=expired", ""); w.Code != 409 {
		t.Fatalf("raw expired cursor %d", w.Code)
	}
	history, _ := g.logs.History("debug", 10)
	if len(history["items"].([]mihomolog.Record)) != 3 {
		t.Fatal("core clear erased page history")
	}
	if w = request("DELETE", "/api/logs/history", ""); w.Code != 200 || cleared.Load() != 1 {
		t.Fatal("history clear touched core")
	}
	if w = request("GET", "/api/logs/history?level=debug&cursor="+page.NextCursor, ""); w.Code != 409 {
		t.Fatalf("stale cursor %d %s", w.Code, w.Body.String())
	}
	if w = request("GET", "/api/logs/history?from=invalid", ""); w.Code != 400 {
		t.Fatal("invalid date accepted")
	}
	if w = request("GET", "/api/logs/history?from=2026-10-10T00:00:00Z&to=2026-10-09T00:00:00Z", ""); w.Code != 400 {
		t.Fatal("reverse range accepted")
	}
	os.Remove(socket)
	if w = request("GET", "/api/logs/settings", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"available":false`) {
		t.Fatal("offline helper status missing")
	}
	if w = request("DELETE", "/api/logs/core", ""); w.Code != 503 {
		t.Fatal("offline clear falsely succeeded")
	}
	if w = request("GET", "/api/logs/core", ""); w.Code != 503 {
		t.Fatal("offline read falsely succeeded")
	}
}
