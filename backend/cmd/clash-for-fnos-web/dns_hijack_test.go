package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
	"gopkg.in/yaml.v3"
)

func TestApplyConfigNormalizesDNSHijackBeforeMihomo(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Payload string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		var config map[string]any
		if err := yaml.Unmarshal([]byte(body.Payload), &config); err != nil {
			t.Error(err)
		}
		want := []any{"114.114.114.114:53", "[2001:db8::1]:53", "tcp://any:5353"}
		if got := config["tun"].(map[string]any)["dns-hijack"]; !reflect.DeepEqual(got, want) {
			t.Errorf("got=%v want=%v", got, want)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	settings := filepath.Join(t.TempDir(), "settings.json")
	data, _ := json.Marshal(map[string]any{"controller": server.URL})
	if err := os.WriteFile(settings, data, 0600); err != nil {
		t.Fatal(err)
	}
	client := &mihomo.Client{SettingsFile: settings}
	if err := applyConfigWithClient(context.Background(), client, []byte("dns: {enable: true}\ntun: {enable: true, dns-hijack: [114.114.114.114, '2001:db8::1', tcp://any:5353]}\n")); err != nil {
		t.Fatal(err)
	}
	if err := applyConfigWithClient(context.Background(), client, []byte("tun: {dns-hijack: [any:53, bad-address]}\n")); err == nil || !strings.Contains(err.Error(), "第 2 项格式无效") {
		t.Fatalf("err=%v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("invalid config was sent to Mihomo")
	}
	// Reject before attempting live Controller discovery or touching files.
	g := &gateway{}
	if _, _, err := g.prepareLiveConfig(context.Background(), []byte("tun: {dns-hijack: [bad-address]}\n")); err == nil || !strings.Contains(err.Error(), "第 1 项格式无效") {
		t.Fatalf("err=%v", err)
	}
}
