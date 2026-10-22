package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBypassMonitorDoesNotCreateOrChangeConfiguration(t *testing.T) {
	h := testHelper(t)
	h.bypassMonitor(context.Background())
	if fileExists(h.config.managedConfig) || fileExists(h.userSettingsPath()) {
		t.Fatal("read created configuration")
	}
	raw := []byte("mode: global\ntun: {enable: false}\ndns: {enable: true, listen: '127.0.0.1:1053'}\nsecret: do-not-return\n")
	if err := atomicWrite(h.config.managedConfig, raw, 0600); err != nil {
		t.Fatal(err)
	}
	result := h.bypassMonitor(context.Background())
	body, _ := json.Marshal(result)
	if strings.Contains(string(body), "do-not-return") {
		t.Fatal("monitor leaked credentials")
	}
	after, _ := os.ReadFile(h.config.managedConfig)
	if string(after) != string(raw) {
		t.Fatal("monitor changed configuration")
	}
	if fileExists(h.userSettingsPath()) {
		t.Fatal("monitor wrote user preferences")
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/network/bypass", strings.NewReader(`{"enabled":true}`)))
	if recorder.Code != 404 {
		t.Fatalf("mutation endpoint still present: %d", recorder.Code)
	}
}
func TestBypassMonitorPreservesUnknownAndFirewallUncertainty(t *testing.T) {
	config, err := monitorConfig([]byte("tun: {enable: true, route-exclude-address: [192.168.0.0/16]}\ndns: {enable: true, listen: '127.0.0.1:1053'}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config["autoRedirect"] != nil || config["autoRoute"] != nil || config["tun"] != true {
		t.Fatalf("invented defaults: %v", config)
	}
	firewall, err := summarizeFirewall([]byte(`{"nftables":[{"chain":{"hook":"forward","policy":"drop"}},{"rule":{"expr":[{"masquerade":null}]}},{"rule":{"expr":[{"drop":null}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if firewall["natRules"] != 1 || firewall["dropRules"] != 1 || firewall["forwardChains"] != 1 {
		t.Fatalf("bad summary: %v", firewall)
	}
	if _, ok := firewall["passed"]; ok {
		t.Fatal("firewall counts presented as forwarding proof")
	}
}
