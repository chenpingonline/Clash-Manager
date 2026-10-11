package main

import (
	"context"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProxyBindAddressValidation(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"*", "*"}, {" 192.168.1.100 ", "192.168.1.100"}, {"127.0.0.1", "127.0.0.1"},
		{"fd00::1", "[fd00::1]"}, {"[::1]", "[::1]"}, {"0.0.0.0", "0.0.0.0"},
	} {
		got, err := normalizeProxyBindAddress(test.input)
		if err != nil || got != test.want {
			t.Fatalf("%q: got %q, err %v", test.input, got, err)
		}
	}
	h := offlineNetworkHelper(t)
	before, _ := os.ReadFile(h.config.managedConfig)
	for _, value := range []any{"", "localhost", "192.168.1.100:7890", "192.168.1.0/24", "999.1.1.1", true, nil} {
		if _, err := h.updateNetwork(context.Background(), map[string]any{"bindAddress": value}); err == nil {
			t.Fatalf("accepted invalid address: %v", value)
		}
	}
	after, _ := os.ReadFile(h.config.managedConfig)
	if string(before) != string(after) || len(h.transactions) != 0 {
		t.Fatal("invalid binding created a transaction or changed the config")
	}
}

func TestProxyBindingPersistenceDisableSubscriptionAndRollback(t *testing.T) {
	h := offlineNetworkHelper(t)
	// No proxy ports: use an arbitrary NAS IP without binding the developer host.
	original := "allow-lan: true\nbind-address: '*'\nexternal-controller: 127.0.0.1:9191\nrules: [MATCH,DIRECT]\n"
	if err := atomicWrite(h.config.managedConfig, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	assertConfig := func(raw string, address string, allow bool) {
		t.Helper()
		var config map[string]any
		if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
			t.Fatal(err)
		}
		if config["bind-address"] != address || config["allow-lan"] != allow || config["external-controller"] != "127.0.0.1:9191" {
			t.Fatalf("unexpected config: %s", raw)
		}
	}
	for _, address := range []string{"192.168.1.100", "[fd00::1]", "127.0.0.1", "[::1]", "*"} {
		prepared, err := h.updateNetwork(context.Background(), map[string]any{"bindAddress": address})
		if err != nil {
			t.Fatal(err)
		}
		if prepared["settings"].(map[string]any)["bindAddress"] != address {
			t.Fatalf("save response lost effective binding: %v", prepared["settings"])
		}
		id := prepared["txId"].(string)
		if _, err = h.activateConfig(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(h.config.managedConfig)
		assertConfig(string(raw), address, true)
		status, err := h.networkStatus(context.Background())
		if err != nil || status["settings"].(map[string]any)["bindAddress"] != address {
			t.Fatalf("binding not returned to UI: %v %v", status, err)
		}
		h.commitConfig(id)
	}
	prepared, err := h.updateNetwork(context.Background(), map[string]any{"bindAddress": "192.168.1.100", "allowLan": false})
	if err != nil {
		t.Fatal(err)
	}
	id := prepared["txId"].(string)
	if _, err = h.activateConfig(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(h.config.managedConfig)
	assertConfig(string(raw), "192.168.1.100", false)
	// A new helper composes fresh subscriptions with the persisted user choices.
	restarted := &helper{config: h.config}
	composed, err := restarted.composeUserSettings(map[string]any{"content": original})
	if err != nil {
		t.Fatal(err)
	}
	assertConfig(composed["content"].(string), "192.168.1.100", false)
	if _, err = h.rollbackConfig(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(h.config.managedConfig)
	assertConfig(string(raw), "*", true)
	settings, _, _ := h.readUserSettings()
	if settings["bind-address"] != "*" || settings["allow-lan"] != nil {
		t.Fatalf("binding preferences not rolled back: %v", settings)
	}
}

func TestIPv6ProxyBindingListenerAddress(t *testing.T) {
	listeners := networkListeners("allow-lan: true\nbind-address: '[::1]'\nmixed-port: 12345\nexternal-controller: 127.0.0.1:9191\n")
	for _, listener := range listeners[1:] {
		if listener.address != "[::1]:12345" {
			t.Fatalf("invalid IPv6 listener: %+v", listener)
		}
	}
}

func TestExplicitLoopbackBindingSurvivesOtherSettingsAndSubscription(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "[::1]"} {
		t.Run(address, func(t *testing.T) {
			h := offlineNetworkHelper(t)
			source := "allow-lan: true\nbind-address: '*'\nexternal-controller: 127.0.0.1:9191\n"
			if err := atomicWrite(h.config.managedConfig, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			for _, patch := range []map[string]any{{"bindAddress": address}, {"core": map[string]any{"ipv6": false}}, {"allowLan": false}, {"allowLan": true}} {
				prepared, err := h.updateNetwork(context.Background(), patch)
				if err != nil {
					t.Fatal(err)
				}
				id := prepared["txId"].(string)
				if _, err = h.activateConfig(context.Background(), id); err != nil {
					t.Fatal(err)
				}
				h.commitConfig(id)
				raw, err := os.ReadFile(h.config.managedConfig)
				if err != nil || proxyBindAddressFromConfig(string(raw)) != address {
					t.Fatalf("explicit binding changed: %s %v", raw, err)
				}
			}
			restarted := &helper{config: h.config}
			result, err := restarted.composeUserSettings(map[string]any{"content": source})
			if err != nil || proxyBindAddressFromConfig(result["content"].(string)) != address {
				t.Fatalf("subscription replaced binding: %v %v", result, err)
			}
		})
	}
}
