package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

func TestListenerReleasePreservesUnchangedAndOldRouting(t *testing.T) {
	old := []byte("mode: direct\nlisteners: [{name: stable, type: mixed, port: 11080}, {name: A, type: mixed, port: 11081}]\n")
	next := []byte("mode: global\nlisteners: [{name: stable, type: mixed, port: 11080}, {name: B, type: mixed, port: 11081}]\n")
	release, err := listenerReleaseConfig(old, next)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := configListeners(release)
	want, _ := configListeners([]byte("listeners: [{name: stable, type: mixed, port: 11080}]"))
	if !reflect.DeepEqual(got, want) || !strings.Contains(string(release), "mode: direct") {
		t.Fatalf("unexpected release: %s", release)
	}
	for _, raw := range [][]byte{old, []byte("listeners: []"), []byte("listeners: [{name: stable, type: mixed, port: 11080}]"), append(append([]byte{}, old...), []byte("log-level: debug\n")...)} {
		release, err := listenerReleaseConfig(old, raw)
		if err != nil || len(release) != 0 {
			t.Fatalf("unnecessary release: %s, %v", release, err)
		}
	}
}

// Model Mihomo's create-before-delete behavior using real TCP/UDP sockets.
// A failed bind still returns HTTP 204, as the Core does.
func TestListenerReloadReleasesPortsAndRecovers(t *testing.T) {
	for _, failStage := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(failStage), func(t *testing.T) {
			tcp, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer tcp.Close()
			address := tcp.Addr().String()
			_, port, _ := net.SplitHostPort(address)
			udp, err := net.ListenPacket("udp", address)
			if err != nil {
				t.Fatal(err)
			}
			defer udp.Close()
			old := []byte("listeners: [{name: A, type: mixed, listen: 127.0.0.1, port: " + port + "}]\n")
			next := []byte("listeners: [{name: B, type: mixed, listen: 127.0.0.1, port: " + port + ", udp: true}]\n")
			type sockets struct {
				tcp net.Listener
				udp net.PacketConn
			}
			live := map[string]sockets{"A": {tcp, udp}}
			defer func() {
				for _, s := range live {
					s.tcp.Close()
					s.udp.Close()
				}
			}()
			calls, conflicts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == failStage {
					http.Error(w, "injected reload failure", 500)
					return
				}
				if r.URL.Path != "/configs" || r.URL.Query().Get("force") != "true" {
					t.Errorf("unexpected request %s", r.URL)
				}
				var body struct {
					Payload string `json:"payload"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				entries, err := configListeners([]byte(body.Payload))
				if err != nil {
					t.Error(err)
					return
				}
				wanted := map[string]bool{}
				for _, e := range entries {
					name := e["name"].(string)
					wanted[name] = true
					if _, ok := live[name]; ok {
						continue
					}
					l, err := net.Listen("tcp", address)
					if err != nil {
						conflicts++
						continue
					}
					u, err := net.ListenPacket("udp", address)
					if err != nil {
						l.Close()
						conflicts++
						continue
					}
					live[name] = sockets{l, u}
				}
				for name, s := range live {
					if !wanted[name] {
						s.tcp.Close()
						s.udp.Close()
						delete(live, name)
					}
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			settings := filepath.Join(t.TempDir(), "settings.json")
			data, _ := json.Marshal(map[string]any{"controller": server.URL})
			if err := os.WriteFile(settings, data, 0600); err != nil {
				t.Fatal(err)
			}
			err = applyConfigWithClient(context.Background(), &mihomo.Client{SettingsFile: settings}, next, old)
			if failStage == 0 {
				if err != nil || calls != 2 {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
				if _, ok := live["B"]; !ok {
					t.Fatal("new listener absent despite HTTP success")
				}
			} else {
				if err == nil {
					t.Fatal("reload failure swallowed")
				}
				if _, ok := live["A"]; !ok {
					t.Fatal("original listeners not restored")
				}
			}
			if conflicts != 0 {
				t.Fatalf("observed %d real bind conflicts", conflicts)
			}
		})
	}
}
