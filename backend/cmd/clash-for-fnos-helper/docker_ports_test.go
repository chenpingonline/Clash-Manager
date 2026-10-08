package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDockerStartupPortsPreserveConfigAndSubscriptions(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	t.Setenv("APP_CONTROLLER_PORT", "19090")
	t.Setenv("APP_MIXED_PORT", "17890")
	h := offlineNetworkHelper(t)
	initial := "# existing subscription\nexternal-controller: 127.0.0.1:9090\nmixed-port: 7890\nsecret: keep-secret\ntun: {enable: false, mtu: 1400}\nrules: [MATCH,DIRECT]\n"
	if err := atomicWrite(h.config.managedConfig, []byte(initial), 0640); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.userSettingsPath(), []byte(`{"mixed-port":7891,"allow-lan":false,"tun":{"mtu":1400}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.applyDockerStartupPorts(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(h.config.managedConfig)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = yaml.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config["external-controller"] != "127.0.0.1:19090" || config["mixed-port"] != 17890 || config["secret"] != "keep-secret" || !strings.Contains(string(raw), "# existing subscription") || config["tun"].(map[string]any)["enable"] != false {
		t.Fatalf("startup changed unrelated configuration: %s", raw)
	}
	result, err := h.composeUserSettings(map[string]any{"content": "external-controller: 127.0.0.1:9090\nmixed-port: 7890\nallow-lan: true\ntun: {mtu: 9000}\nrules: [MATCH,DIRECT]\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal([]byte(result["content"].(string)), &config); err != nil {
		t.Fatal(err)
	}
	if config["external-controller"] != "127.0.0.1:19090" || config["mixed-port"] != 17890 || config["allow-lan"] != false || config["tun"].(map[string]any)["mtu"] != 1400 {
		t.Fatalf("subscription lost saved overrides: %v", config)
	}
	// Removing the env override preserves the last saved deployment ports.
	t.Setenv("APP_CONTROLLER_PORT", "")
	t.Setenv("APP_MIXED_PORT", "")
	before, _ := os.ReadFile(h.config.managedConfig)
	if err = h.applyDockerStartupPorts(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(h.config.managedConfig)
	if string(before) != string(after) {
		t.Fatal("empty overrides changed saved configuration")
	}
}

func TestDockerStartupPortsFreshConfigAndNativeIsolation(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	t.Setenv("APP_CONTROLLER_PORT", "19090")
	t.Setenv("APP_MIXED_PORT", "")
	h := testHelper(t)
	if err := h.applyDockerStartupPorts(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(h.config.managedConfig)
	if !strings.Contains(string(raw), "127.0.0.1:19090") || !strings.Contains(string(raw), "mixed-port: 7890") {
		t.Fatalf("fresh config did not use override: %s", raw)
	}
	t.Setenv("APP_PLATFORM", "fnos")
	t.Setenv("APP_CONTROLLER_PORT", "invalid")
	native := testHelper(t)
	if err := native.applyDockerStartupPorts(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(native.config.managedConfig); !os.IsNotExist(err) {
		t.Fatal("Docker override touched native config")
	}
}

func TestInvalidDockerStartupPortsDoNotWrite(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	for _, value := range []string{"0", "-1", "65536", "1.5", "http://127.0.0.1:19090"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APP_CONTROLLER_PORT", "19090")
			t.Setenv("APP_MIXED_PORT", value)
			h := offlineNetworkHelper(t)
			before, _ := os.ReadFile(h.config.managedConfig)
			if err := h.applyDockerStartupPorts(); err == nil || !strings.Contains(err.Error(), "APP_MIXED_PORT") {
				t.Fatalf("invalid port accepted: %v", err)
			}
			after, _ := os.ReadFile(h.config.managedConfig)
			if string(before) != string(after) {
				t.Fatal("invalid override changed config")
			}
			if _, err := os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
				t.Fatal("invalid override saved preferences")
			}
		})
	}
}
