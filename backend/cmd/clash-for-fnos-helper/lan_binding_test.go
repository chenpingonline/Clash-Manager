package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func assertLANConfig(t *testing.T, raw string, enabled bool, bind string) {
	t.Helper()
	var config map[string]any
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	gotBind, _ := config["bind-address"].(string)
	if config["allow-lan"] != enabled || gotBind != bind {
		t.Fatalf("LAN settings: allow-lan=%v bind-address=%v; want %t %q", config["allow-lan"], config["bind-address"], enabled, bind)
	}
	if config["external-controller"] != "127.0.0.1:9191" {
		t.Fatalf("LAN switch changed Controller API: %v", config["external-controller"])
	}
}

func assertIPv6Config(t *testing.T, raw string, enabled bool) {
	t.Helper()
	var config map[string]any
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	dns, _ := config["dns"].(map[string]any)
	if config["ipv6"] != enabled || dns["ipv6"] != true {
		t.Fatalf("global IPv6 must be independent from LAN and DNS IPv6: %s", raw)
	}
}

func TestLANBindingActivationSubscriptionAndRollback(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			h := offlineNetworkHelper(t)
			before, err := os.ReadFile(h.config.managedConfig)
			if err != nil {
				t.Fatal(err)
			}
			previousBind := "127.0.0.1"
			if !enabled {
				previousBind = "*"
			}
			before = append(before, []byte(fmt.Sprintf("allow-lan: %t\nbind-address: %q\n", !enabled, previousBind))...)
			if err = atomicWrite(h.config.managedConfig, before, 0600); err != nil {
				t.Fatal(err)
			}
			prepared, err := h.updateNetwork(context.Background(), map[string]any{"allowLan": enabled})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
				t.Fatal("LAN preferences saved before activation")
			}
			id := prepared["txId"].(string)
			if _, err = h.activateConfig(context.Background(), id); err != nil {
				t.Fatal(err)
			}
			bind := "*"
			active, err := os.ReadFile(h.config.managedConfig)
			if err != nil {
				t.Fatal(err)
			}
			assertLANConfig(t, string(active), enabled, bind)
			settings, _, err := h.readUserSettings()
			if err != nil || settings["allow-lan"] != enabled || settings["bind-address"] != nil {
				t.Fatalf("LAN preference must not override custom subscription bindings: %v %v", settings, err)
			}
			// A fresh helper applies the LAN switch, then checks each subscription binding.
			restarted := &helper{config: h.config}
			for _, binding := range []string{"", "bind-address: 127.0.0.1\n", "bind-address: 192.168.1.2\n"} {
				source := fmt.Sprintf("allow-lan: %t\nexternal-controller: 127.0.0.1:9191\nrules: [MATCH,DIRECT]\n%s", !enabled, binding)
				result, err := restarted.composeUserSettings(map[string]any{"content": source})
				if err != nil {
					t.Fatal(err)
				}
				wantBind := ""
				if binding != "" {
					wantBind = "127.0.0.1"
					if enabled {
						wantBind = "*"
					}
					if strings.Contains(binding, "192.168.1.2") {
						wantBind = "192.168.1.2"
					}
				}
				assertLANConfig(t, result["content"].(string), enabled, wantBind)
			}
			if _, err = h.rollbackConfig(context.Background(), id); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(h.config.managedConfig)
			if err != nil || string(after) != string(before) {
				t.Fatalf("LAN config rollback failed: %s %v", after, err)
			}
			if _, err = os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
				t.Fatal("LAN preference rollback failed")
			}
		})
	}
}

func TestLANCustomBindingSurvivesToggle(t *testing.T) {
	for _, bind := range []string{"192.168.1.2", "[fd00::1]", "0.0.0.0", "*"} {
		t.Run(bind, func(t *testing.T) {
			h := offlineNetworkHelper(t)
			// No proxy port is enabled: these custom interface addresses need not
			// exist on the test host in order to verify that they are preserved.
			source := fmt.Sprintf("allow-lan: false\nbind-address: %q\nexternal-controller: 127.0.0.1:9191\n", bind)
			if err := atomicWrite(h.config.managedConfig, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{true, false, true} {
				prepared, err := h.updateNetwork(context.Background(), map[string]any{"allowLan": enabled})
				if err != nil {
					t.Fatal(err)
				}
				id := prepared["txId"].(string)
				if _, err = h.activateConfig(context.Background(), id); err != nil {
					t.Fatal(err)
				}
				h.commitConfig(id)
				raw, err := os.ReadFile(h.config.managedConfig)
				if err != nil {
					t.Fatal(err)
				}
				assertLANConfig(t, string(raw), enabled, bind)
			}
		})
	}
}

func TestLANAndIPv6SwitchesRemainIndependent(t *testing.T) {
	for _, lan := range []bool{true, false} {
		for _, ipv6 := range []bool{true, false} {
			t.Run(fmt.Sprintf("lan=%t/ipv6=%t", lan, ipv6), func(t *testing.T) {
				h := offlineNetworkHelper(t)
				source := "allow-lan: false\nbind-address: 127.0.0.1\nexternal-controller: 127.0.0.1:9191\ndns: {enable: true, ipv6: true}\n"
				if err := atomicWrite(h.config.managedConfig, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				prepared, err := h.updateNetwork(context.Background(), map[string]any{"allowLan": lan, "core": map[string]any{"ipv6": ipv6}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = h.activateConfig(context.Background(), prepared["txId"].(string)); err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(h.config.managedConfig)
				if err != nil {
					t.Fatal(err)
				}
				bind := "127.0.0.1"
				if lan {
					bind = "*"
				}
				assertLANConfig(t, string(raw), lan, bind)
				assertIPv6Config(t, string(raw), ipv6)
				restarted := &helper{config: h.config}
				composed, err := restarted.composeUserSettings(map[string]any{"content": source})
				if err != nil {
					t.Fatal(err)
				}
				content := composed["content"].(string)
				assertLANConfig(t, content, lan, bind)
				assertIPv6Config(t, content, ipv6)
			})
		}
	}
}

func TestLegacyLANPreferencesOverrideSubscriptionBinding(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			h := offlineNetworkHelper(t)
			if err := atomicWrite(h.userSettingsPath(), []byte(fmt.Sprintf(`{"allow-lan":%t}`, enabled)), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := h.composeUserSettings(map[string]any{"content": "allow-lan: true\nbind-address: 127.0.0.1\nexternal-controller: 127.0.0.1:9191\n"})
			if err != nil {
				t.Fatal(err)
			}
			bind := "127.0.0.1"
			if enabled {
				bind = "*"
			}
			assertLANConfig(t, result["content"].(string), enabled, bind)
		})
	}
}

func TestUntouchedLANBindingIsPreserved(t *testing.T) {
	h := offlineNetworkHelper(t)
	source := "allow-lan: true\nbind-address: 192.168.1.2\nexternal-controller: 127.0.0.1:9191\n"
	if err := atomicWrite(h.userSettingsPath(), []byte(`{"ipv6":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := h.composeUserSettings(map[string]any{"content": source})
	if err != nil {
		t.Fatal(err)
	}
	assertLANConfig(t, result["content"].(string), true, "192.168.1.2")
	// Saving an unrelated setting must also retain an explicit startup binding.
	source = "mixed-port: 7890\nallow-lan: false\nbind-address: 192.168.1.2\nexternal-controller: 127.0.0.1:9191\n"
	if err := atomicWrite(h.config.managedConfig, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := h.updateNetwork(context.Background(), map[string]any{"core": map[string]any{"ipv6": false}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.activateConfig(context.Background(), prepared["txId"].(string)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(h.config.managedConfig)
	if err != nil {
		t.Fatal(err)
	}
	assertLANConfig(t, string(raw), false, "192.168.1.2")
}

func TestInvalidLANSettingAndValidationFailureDoNotPersist(t *testing.T) {
	h := offlineNetworkHelper(t)
	before, err := os.ReadFile(h.config.managedConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{"true", float64(1), nil, map[string]any{}} {
		if _, err = h.updateNetwork(context.Background(), map[string]any{"allowLan": value}); err == nil {
			t.Fatalf("accepted invalid LAN setting: %v", value)
		}
	}
	if err = atomicWrite(h.config.managedCore, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = h.updateNetwork(context.Background(), map[string]any{"allowLan": true}); err == nil {
		t.Fatal("accepted invalid Core config")
	}
	after, err := os.ReadFile(h.config.managedConfig)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed LAN save changed startup config", err)
	}
	if _, err = os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
		t.Fatal("failed LAN save persisted preferences")
	}
}
