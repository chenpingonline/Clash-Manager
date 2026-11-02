package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

// Opt-in against an isolated Mihomo with an UDP echo service at 127.0.0.1:11000.
// The three addresses may use different host ports when published by Docker.
func TestMihomoListenerProfileSwitchIntegration(t *testing.T) {
	controller := os.Getenv("CLASH_TEST_MIHOMO_CONTROLLER")
	if controller == "" {
		t.Skip("requires isolated Mihomo and UDP echo service")
	}
	tcpAddress, udpAddress := os.Getenv("CLASH_TEST_MIHOMO_TCP"), os.Getenv("CLASH_TEST_MIHOMO_UDP")
	settings := filepath.Join(t.TempDir(), "settings.json")
	data, _ := json.Marshal(map[string]any{"controller": controller})
	if err := os.WriteFile(settings, data, 0600); err != nil {
		t.Fatal(err)
	}
	client := &mihomo.Client{SettingsFile: settings}
	base := "external-controller: 0.0.0.0:9090\nmode: rule\nipv6: false\nrules: ['MATCH,DIRECT']\n"
	manual := []byte(base + "listeners: [{name: manual, type: mixed, listen: 0.0.0.0, port: 1082, proxy: DIRECT}]\n")
	a := []byte(base + "listeners: [{name: managed-A, type: mixed, listen: 0.0.0.0, port: 1082, proxy: DIRECT, udp: true}]\n")
	b := []byte(base + "listeners: [{name: managed-B, type: mixed, listen: 0.0.0.0, port: 1082, proxy: DIRECT, udp: true}]\n")
	ctx := context.Background()
	if err := restoreConfigListeners(ctx, client, manual); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := putRuntimeConfig(ctx, client, []byte(base+"listeners: []\n")); err != nil {
			t.Error(err)
		}
	}()
	check := func() {
		t.Helper()
		proxy, _ := url.Parse("http://" + tcpAddress)
		transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
		defer transport.CloseIdleConnections()
		response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Get("http://127.0.0.1:9090/version")
		if err != nil {
			t.Fatalf("HTTP proxy unavailable: %v", err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || !bytes.Contains(body, []byte("version")) {
			t.Fatalf("HTTP proxy response: %s", body)
		}
		conn, err := net.DialTimeout("tcp", tcpAddress, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err = conn.Write([]byte{5, 1, 0}); err != nil {
			t.Fatal(err)
		}
		reply := make([]byte, 2)
		if _, err = io.ReadFull(conn, reply); err != nil || !bytes.Equal(reply, []byte{5, 0}) {
			t.Fatalf("SOCKS greeting: %v %v", reply, err)
		}
		if _, err = conn.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
			t.Fatal(err)
		}
		header := make([]byte, 4)
		if _, err = io.ReadFull(conn, header); err != nil || header[1] != 0 {
			t.Fatalf("UDP associate: %v %v", header, err)
		}
		size := 6
		if header[3] == 4 {
			size = 18
		}
		if header[3] == 3 {
			t.Fatal("unexpected domain UDP relay")
		}
		if _, err = io.ReadFull(conn, make([]byte, size)); err != nil {
			t.Fatal(err)
		}
		packet, err := net.Dial("udp", udpAddress)
		if err != nil {
			t.Fatal(err)
		}
		defer packet.Close()
		packet.SetDeadline(time.Now().Add(3 * time.Second))
		payload := []byte("issue21-udp-echo")
		// SOCKS5 UDP header followed by 127.0.0.1:11000 and the echo payload.
		request := append([]byte{0, 0, 0, 1, 127, 0, 0, 1, 42, 248}, payload...)
		if _, err = packet.Write(request); err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, 2048)
		n, err := packet.Read(buffer)
		if err != nil || !bytes.HasSuffix(buffer[:n], payload) {
			t.Fatalf("UDP proxy unavailable: %v (%x)", err, buffer[:n])
		}
	}
	check()
	swapA := []byte(base + "listeners: [{name: first, type: mixed, listen: 0.0.0.0, port: 1082, proxy: DIRECT}, {name: second, type: mixed, listen: 0.0.0.0, port: 1083, proxy: DIRECT}]\n")
	swapB := bytes.ReplaceAll(bytes.ReplaceAll(bytes.ReplaceAll(swapA, []byte("1082"), []byte("SWAP")), []byte("1083"), []byte("1082")), []byte("SWAP"), []byte("1083"))
	previous := manual
	for _, next := range [][]byte{a, b, a, manual, b, manual, a, swapA, swapB, swapA} {
		if err := applyConfigWithClient(ctx, client, next, previous); err != nil {
			t.Fatal(err)
		}
		check()
		previous = next
	}
	// A rejected final candidate must restore functioning original TCP and UDP.
	invalid := bytes.Replace(b, []byte("type: mixed"), []byte("type: invalid-protocol"), 1)
	if err := applyConfigWithClient(ctx, client, invalid, previous); err == nil {
		t.Fatal("invalid candidate accepted")
	}
	check()
	t.Log("10 profile switches (including port swaps) and failed-candidate recovery passed real HTTP/SOCKS5 UDP traffic")
}
