package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGroupDelaySettingsAndResultsAreScoped(t *testing.T) {
	var mu sync.Mutex
	queries := []string{}
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxies" {
			_, _ = io.WriteString(w, `{"proxies":{"AI":{"all":["same"],"testUrl":"https://ai.example/test","expectedStatus":"200"},"Video":{"all":["same"],"testUrl":"https://video.example/test"}}}`)
			return
		}
		if r.URL.Path == "/proxies/same/delay" {
			mu.Lock()
			queries = append(queries, r.URL.Query().Get("url")+"|"+r.URL.Query().Get("expected"))
			mu.Unlock()
			_, _ = io.WriteString(w, `{"delay":23}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer controller.Close()
	dir := t.TempDir()
	cfg := config{settingsFile: writeGatewaySettings(t, controller.URL), configMetaFile: filepath.Join(dir, "meta.json")}
	g := newGateway(cfg)
	activate := func(id string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"source": "profile", "sourceId": id, "active": true})
		if err := os.WriteFile(cfg.configMetaFile, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := func(method, path string, body any, code int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(raw)))
		if w.Code != code {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	current := func() groupDelayContext {
		t.Helper()
		var result struct {
			Context groupDelayContext `json:"delayContext"`
		}
		if err := json.Unmarshal(request("GET", "/api/proxies", nil, 200), &result); err != nil {
			t.Fatal(err)
		}
		return result.Context
	}
	wait := func() delayBatchStatus {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var status delayBatchStatus
			_ = json.Unmarshal(request("GET", "/api/delays", nil, 200), &status)
			if status.State == "done" {
				return status
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("test did not complete")
		return delayBatchStatus{}
	}
	save := func(scope, group, mode, target string, code int) {
		t.Helper()
		request("PUT", "/api/delay-settings", map[string]string{"scope": scope, "group": group, "mode": mode, "url": target}, code)
	}
	activate("a")
	a := current()
	if a.Groups["AI"].URL != "https://ai.example/test" || a.Groups["AI"].Expected != "200" {
		t.Fatalf("missing group config: %#v", a)
	}
	save(a.Scope, "AI", "custom", "https://custom.example/test", 200)
	a = current()
	if a.Groups["AI"].Mode != "custom" || a.Groups["AI"].URL != "https://custom.example/test" || a.Groups["AI"].Expected != "" {
		t.Fatalf("custom: %#v", a)
	}
	request("POST", "/api/delays", delayBatchRequest{Scope: a.Scope, Groups: []delayGroupRequest{{Group: "AI", Names: []string{"same"}}, {Group: "Video", Names: []string{"same"}}}}, 202)
	first := wait()
	if len(first.Results) != 2 || len(first.Targets) != 2 {
		t.Fatalf("collapsed different targets: %#v", first)
	}
	activate("b")
	b := current()
	if b.Scope == a.Scope || b.Groups["AI"].Mode != "auto" || b.Groups["AI"].URL != "https://ai.example/test" {
		t.Fatalf("subscription leaked: %#v", b)
	}
	save(a.Scope, "AI", "global", "", 409)
	request("POST", "/api/delays", delayBatchRequest{Scope: a.Scope, Group: "AI", Names: []string{"same"}}, 409)
	request("POST", "/api/delays", delayBatchRequest{Scope: b.Scope, Group: "AI", Names: []string{"not-member"}}, 400)
	request("POST", "/api/delays", delayBatchRequest{Scope: b.Scope, Group: "AI", Names: []string{"same"}}, 202)
	second := wait()
	if len(second.Results) != 3 {
		t.Fatalf("result scopes collided: %#v", second)
	}
	mu.Lock()
	got := strings.Join(queries, ",")
	mu.Unlock()
	if !strings.Contains(got, "https://ai.example/test|200") || !strings.Contains(got, "https://custom.example/test|") || !strings.Contains(got, "https://video.example/test|") {
		t.Fatalf("incorrect target queries: %s", got)
	}
	activate("a")
	g = newGateway(cfg)
	a = current()
	if a.Groups["AI"].URL != "https://custom.example/test" {
		t.Fatal("setting did not persist")
	}
	save(a.Scope, "AI", "global", "", 200)
	a = current()
	if a.Groups["AI"].URL != a.GlobalURL {
		t.Fatal("global override failed")
	}
	save(a.Scope, "AI", "auto", "", 200)
	a = current()
	if a.Groups["AI"].URL != "https://ai.example/test" {
		t.Fatal("reset did not restore group config")
	}
	save(a.Scope, "AI", "custom", "ftp://invalid.example", 400)
	save(a.Scope, "AI", "custom", "https://user:password@example.com/", 400)
}

func TestGroupDelayUsesExplicitYAMLAndGlobalFallback(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "group-delay-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	helper := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config/active-raw" {
			writeJSON(w, 200, map[string]any{"content": "proxy-groups:\n- name: AI\n  type: select\n  url: https://explicit.example/\n  expected-status: 204\n- name: Video\n  type: select\n"})
			return
		}
		http.NotFound(w, r)
	})}
	go helper.Serve(listener)
	defer helper.Close()
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"proxies":{"AI":{"all":["same"]},"Video":{"all":["same"],"testUrl":"https://implicit.example/"}}}`)
	}))
	defer controller.Close()
	g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: socket})
	recorder := httptest.NewRecorder()
	g.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/proxies", nil))
	if recorder.Code != 200 {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Context groupDelayContext `json:"delayContext"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
	if payload.Context.Groups["AI"].URL != "https://explicit.example/" || payload.Context.Groups["AI"].Expected != "204" {
		t.Fatalf("YAML not respected: %#v", payload)
	}
	if payload.Context.Groups["Video"].URL != payload.Context.GlobalURL || payload.Context.Groups["Video"].ConfigURL != "" {
		t.Fatalf("implicit Core URL replaced global fallback: %#v", payload)
	}
}

func TestDelayCompletionDiscardsMeasurementDuringConfigurationSwitch(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, `{"delay":10}`)
	}))
	defer controller.Close()
	g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL)})
	start := httptest.NewRecorder()
	g.ServeHTTP(start, httptest.NewRequest("POST", "/api/delays", strings.NewReader(`{"names":["same"]}`)))
	if start.Code != 202 {
		t.Fatalf("%d %s", start.Code, start.Body.String())
	}
	<-entered
	g.delayGeneration.Add(1)
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := httptest.NewRecorder()
		g.ServeHTTP(status, httptest.NewRequest("GET", "/api/delays", nil))
		var result delayBatchStatus
		_ = json.Unmarshal(status.Body.Bytes(), &result)
		if result.State == "done" {
			if len(result.Results) != 1 || result.Results[0].Delay != 0 || result.Results[0].State != "error" {
				t.Fatalf("stale success: %#v", result)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("test did not finish")
}
