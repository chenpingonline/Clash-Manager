package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNetworkStatusReadsDNSHijackLists(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"block", "tun:\n  dns-hijack:\n    - any:53\n    - tcp://any:53\n", true},
		{"flow list", "tun:\n  dns-hijack: [any:53, 'tcp://any:53']\n", true},
		{"flow mapping", "tun: {enable: true, dns-hijack: [any:53]}\n", true},
		{"custom target", "tun:\n    dns-hijack:\n    - 192.168.0.1:53\n", true},
		{"alias", "targets: &targets [any:53]\ntun: {dns-hijack: *targets}\n", true},
		{"empty", "tun: {dns-hijack: []}\n", false},
		{"missing", "tun: {enable: true}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := offlineNetworkHelper(t)
			if err := atomicWrite(h.config.managedConfig, []byte("dns: {enable: true}\n"+tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			status, err := h.networkStatus(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings := status["settings"].(map[string]any)
			if got := settings["tun"].(map[string]any)["dnsHijack"]; got != tc.want {
				t.Fatalf("dnsHijack=%v want %v", got, tc.want)
			}
			if settings["dnsEnabled"] != true {
				t.Fatalf("actual DNS state=%v", settings["dnsEnabled"])
			}
		})
	}
}

func TestNetworkDNSHijackSaveReloadAndDisable(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	if err := atomicWrite(h.config.managedConfig, []byte("dns: {enable: true}\ntun: {enable: true, dns-hijack: [], device: keep-tun}\nrules: ['MATCH,DIRECT']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input map[string]any
		want  bool
	}{
		{map[string]any{"dnsHijack": true}, true},
		{map[string]any{"mtu": float64(1400)}, true},
		{map[string]any{"dnsHijack": false}, false},
	} {
		prepared, err := h.updateNetwork(ctx, map[string]any{"tun": tc.input})
		if err != nil {
			t.Fatal(err)
		}
		id := prepared["txId"].(string)
		if _, err := h.activateConfig(ctx, id); err != nil {
			t.Fatal(err)
		}
		if _, err := h.commitConfig(id); err != nil {
			t.Fatal(err)
		}
		status, err := h.networkStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got := status["settings"].(map[string]any)["tun"].(map[string]any)["dnsHijack"]; got != tc.want {
			t.Fatalf("reloaded dnsHijack=%v want %v", got, tc.want)
		}
		raw, _ := os.ReadFile(h.config.managedConfig)
		var document map[string]any
		if err := yaml.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		tun := document["tun"].(map[string]any)
		want := []any{}
		if tc.want {
			want = []any{"any:53", "tcp://any:53"}
		}
		if !reflect.DeepEqual(tun["dns-hijack"], want) {
			t.Fatalf("targets=%v want %v", tun["dns-hijack"], want)
		}
		if tun["device"] != "keep-tun" {
			t.Fatalf("lost custom device: %v", tun)
		}
		prefs, _, err := h.readUserSettings()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(prefs["tun"].(map[string]any)["dns-hijack"], want) {
			t.Fatalf("persistent override=%v", prefs)
		}
	}
}

func TestNetworkDNSHijackRequiresEnabledDNS(t *testing.T) {
	for _, dns := range []string{"", "dns: {enable: false}\n", "dns:\n    enable: false\n"} {
		h := offlineNetworkHelper(t)
		original := []byte(dns + "tun: {enable: true, dns-hijack: []}\n")
		if err := atomicWrite(h.config.managedConfig, original, 0600); err != nil {
			t.Fatal(err)
		}
		_, err := h.updateNetwork(context.Background(), map[string]any{"tun": map[string]any{"dnsHijack": true}})
		if err == nil || !strings.Contains(err.Error(), "需要先启用 Mihomo DNS") {
			t.Fatalf("expected prerequisite error, got %v", err)
		}
		after, _ := os.ReadFile(h.config.managedConfig)
		if string(after) != string(original) {
			t.Fatal("rejected save changed config")
		}
		if len(h.transactions) != 0 {
			t.Fatal("rejected save created a transaction")
		}
	}
}

func TestTunDNSGuardKeepsExplicitChoiceAndSupportsInlineDNS(t *testing.T) {
	for _, dns := range []string{"dns: {enable: true}\n", "dns:\n    enable: true\n"} {
		raw := dns + "tun:\n  dns-hijack: [any:53, tcp://any:53]\n"
		got, changed, err := guardTunDNSHijack(raw, true)
		if err != nil || changed || got != raw {
			t.Fatalf("got=%q changed=%v err=%v", got, changed, err)
		}
	}
	// A fresh TUN without an explicit hijack choice stays safe with DNS disabled.
	got, changed, err := guardTunDNSHijack("tun: {enable: false}\n", true)
	if err != nil || !changed || tunDNSHijackEnabled(got) {
		t.Fatalf("got=%q changed=%v err=%v", got, changed, err)
	}
}

func TestDNSHijackPreconfigureAndDNSDisableGuard(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	// Preconfiguration remains allowed before DNS and TUN are enabled.
	prepared, err := h.updateNetwork(ctx, map[string]any{"tun": map[string]any{"dnsHijack": true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.activateConfig(ctx, prepared["txId"].(string)); err != nil {
		t.Fatal(err)
	}
	status, err := h.networkStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings := status["settings"].(map[string]any)
	if settings["dnsEnabled"] != false || settings["tun"].(map[string]any)["dnsHijack"] != true {
		t.Fatalf("lost preconfiguration: %v", settings)
	}
	// Turning DNS off while hijack is active must not create a broken candidate.
	if err := atomicWrite(h.config.managedConfig, []byte("dns: {enable: true}\ntun: {enable: true, dns-hijack: [any:53]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = h.updateNetwork(ctx, map[string]any{"dns": map[string]any{"enable": false}})
	if err == nil || !strings.Contains(err.Error(), "需要先启用 Mihomo DNS") {
		t.Fatalf("accepted disabling required DNS: %v", err)
	}
}

func TestDNSHijackNormalizationBeforeConfigTransaction(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	raw := "dns: {enable: true}\ntun:\n  enable: false\n  dns-hijack: [114.114.114.114, '2001:db8::1', tcp://any:5353]\n"
	// The local check also runs when Mihomo's validation is skipped for live apply.
	prepared, err := h.prepareConfigCandidate(ctx, raw, false, true)
	if err != nil {
		t.Fatal(err)
	}
	effective := prepared["effectiveContent"].(string)
	for _, want := range []string{"114.114.114.114:53", "[2001:db8::1]:53", "tcp://any:5353"} {
		if !strings.Contains(effective, want) {
			t.Fatalf("missing %q in %s", want, effective)
		}
	}
	if _, err := h.validateConfigTransaction(ctx, prepared["txId"].(string)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.activateConfig(ctx, prepared["txId"].(string)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(h.config.managedConfig)
	count := len(h.transactions)
	for _, validate := range []bool{true, false} {
		_, err := h.prepareConfigCandidate(ctx, "tun: {dns-hijack: [any:53, bad-address]}\n", validate, false)
		if err == nil || !strings.Contains(err.Error(), "第 2 项格式无效") {
			t.Fatalf("err=%v", err)
		}
	}
	after, _ := os.ReadFile(h.config.managedConfig)
	if string(before) != string(after) || len(h.transactions) != count {
		t.Fatal("invalid config changed target or created transaction")
	}
}

func TestDNSHijackGuardNormalizesBeforeCheckingDNS(t *testing.T) {
	raw := "dns: {enable: true}\ntun: {dns-hijack: [114.114.114.114]}\n"
	got, disabled, err := guardTunDNSHijack(raw, true)
	if err != nil || disabled || !strings.Contains(got, "114.114.114.114:53") {
		t.Fatalf("got=%s disabled=%v err=%v", got, disabled, err)
	}
	// A missing DNS module must not cause malformed hijack values to be silently cleared.
	for _, value := range []string{"true", "[bad-address]"} {
		_, _, err := guardTunDNSHijack("tun: {dns-hijack: "+value+"}\n", true)
		if err == nil || !strings.Contains(err.Error(), "tun.dns-hijack") {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestDNSHijackNormalizationAfterSubscriptionOverrides(t *testing.T) {
	h := offlineNetworkHelper(t)
	if err := atomicWrite(h.userSettingsPath(), []byte(`{"tun":{"dns-hijack":["114.114.114.114"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := h.composeUserSettings(map[string]any{"content": "tun: {enable: false, dns-hijack: [invalid]}\nrules: []\n"})
	if err != nil || !strings.Contains(result["content"].(string), "114.114.114.114:53") {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if err := atomicWrite(h.userSettingsPath(), []byte(`{"tun":{"dns-hijack":["invalid"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.composeUserSettings(map[string]any{"content": "tun: {enable: false}\n"}); err == nil || !strings.Contains(err.Error(), "第 1 项格式无效") {
		t.Fatalf("err=%v", err)
	}
}
