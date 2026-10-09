package main

import (
	"context"
	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"strings"
	"testing"
)

func TestTunVersionFeatures(t *testing.T) {
	for _, tc := range []struct {
		version, stack   string
		mips, congestion bool
	}{
		{"v1.19.30", "gvisor", false, false}, {"1.19.31", "gvisor", true, false},
		{"v1.19.32", "mips", true, true}, {"v1.20.0", "mips", true, true}, {"", "", false, false},
	} {
		got := tunVersionFeatures(tc.version)
		if got["defaultStack"] != tc.stack || got["mips"] != tc.mips || got["congestionController"] != tc.congestion {
			t.Fatalf("%s: %v", tc.version, got)
		}
	}
}

func TestTunDefaultRemovesOnlyExplicitOverrides(t *testing.T) {
	for _, raw := range []string{"tun: {enable: true, stack: mixed, congestion-controller: bbr, mtu: 1400, device: custom}\n", "rules: [MATCH,DIRECT]\n"} {
		tun, err := normalizeTunForYAML(map[string]any{"stack": "", "congestionController": ""})
		if err != nil {
			t.Fatal(err)
		}
		// Exercise the same API-to-YAML mapping and merge as the save path.
		out, err := configyaml.MergeOverrides([]byte(raw), map[string]any{"tun": map[string]any{"stack": tun["stack"], "congestion-controller": tun["congestionController"]}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "stack:") || strings.Contains(string(out), "congestion-controller:") {
			t.Fatalf("defaults left overrides: %s", out)
		}
		if strings.Contains(raw, "custom") && (!strings.Contains(string(out), "custom") || !strings.Contains(string(out), "1400")) {
			t.Fatalf("lost other fields: %s", out)
		}
	}
}

func TestOfflineTunReportsUnspecifiedAndExplicitStack(t *testing.T) {
	h := offlineNetworkHelper(t)
	for _, stack := range []string{"", "mixed", "mips"} {
		raw := "mixed-port: 7890\nexternal-controller: 127.0.0.1:9191\ntun:\n  enable: false\n"
		if stack != "" {
			raw += "  stack: " + stack + "\n  congestion-controller: bbr3\n"
		}
		if err := atomicWrite(h.config.managedConfig, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := h.networkStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		tun := got["settings"].(map[string]any)["tun"].(map[string]any)
		if tun["stack"] != stack {
			t.Fatalf("stack=%s: %v", stack, tun)
		}
		if stack != "" && tun["congestionController"] != "bbr3" {
			t.Fatalf("lost congestion: %v", tun)
		}
	}
}

func TestTunDefaultPatchSurvivesSubscriptionReapply(t *testing.T) {
	patch, err := normalizeUserPatch(map[string]any{"tun": map[string]any{"stack": "", "congestionController": ""}})
	if err != nil {
		t.Fatal(err)
	}
	existing := map[string]any{"tun": map[string]any{"stack": "mixed", "congestion-controller": "bbr", "mtu": 1400}}
	mergeUserSettings(existing, patch)
	out, err := configyaml.MergeOverrides([]byte("tun: {enable: true, stack: system, device: keep}\n"), existing)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "stack:") || strings.Contains(string(out), "congestion-controller:") || !strings.Contains(string(out), "device: keep") || !strings.Contains(string(out), "mtu: 1400") {
		t.Fatalf("%s", out)
	}
	patch, err = normalizeUserPatch(map[string]any{"tun": map[string]any{"mtu": 1500}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = configyaml.MergeOverrides([]byte("tun: {stack: mixed, congestion-controller: bbr3}\n"), patch)
	if err != nil || !strings.Contains(string(out), "stack: mixed") || !strings.Contains(string(out), "congestion-controller: bbr3") {
		t.Fatalf("%s %v", out, err)
	}
}
