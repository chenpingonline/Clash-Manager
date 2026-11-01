package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedLogDisplayAcrossClientsAndRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := config{settingsFile: filepath.Join(dir, "settings.json"), mihomoLogFile: filepath.Join(dir, "mihomo.log"), privilegedSocket: filepath.Join(dir, "unavailable.sock")}
	g := newGateway(cfg)
	request := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(method, "/api/logs/display", strings.NewReader(body)))
		return w
	}
	if w := request("GET", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"limit":100`) {
		t.Fatalf("initial %d %s", w.Code, w.Body.String())
	}
	if w := request("PUT", `{"limit":500}`); w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	// A fresh gateway has no client-side state and reads the same persisted value.
	g = newGateway(cfg)
	if w := request("GET", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"limit":500`) {
		t.Fatalf("second client %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{`{}`, `{"limit":0}`, `{"limit":999999}`, `{"limit":"200"}`, `{`} {
		if w := request("PUT", body); w.Code != 400 {
			t.Fatalf("invalid accepted %q %d", body, w.Code)
		}
	}
	if w := request("GET", ""); !strings.Contains(w.Body.String(), `"limit":500`) {
		t.Fatal("invalid request changed settings")
	}
	if w := request("DELETE", ""); w.Code != 405 {
		t.Fatal("unexpected delete support")
	}
}
