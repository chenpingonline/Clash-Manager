package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveNetAdmin(t *testing.T) {
	for _, item := range []struct {
		status string
		want   bool
	}{
		{"Uid:\t0 0 0 0\nCapEff:\t0000000000000000\n", false},
		{"CapEff:\t0000000000001000\n", true},
		{"CapEff:\t0000000000002000\n", false},
		{"CapEff:\tinvalid\n", false},
		{"", false},
	} {
		if got := hasNetAdmin(item.status); got != item.want {
			t.Fatalf("%q: %v", item.status, got)
		}
	}
}
func TestDockerRootDoesNotImplyNetworkPermission(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	result := resolveTunCapability(&processInfo{PID: -1, Managed: true}, true, 0)
	if result["supported"] != false || result["permission"] != false {
		t.Fatal("Docker root accepted without actual capability")
	}
}
func TestDockerBlocksNativeHelperOperations(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	h := newHelper(helperConfig{})
	for _, path := range []string{"/app/icon/update", "/system/proxy-environment/update"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(`{}`)))
		if rec.Code != 409 {
			t.Fatalf("%s returned %d", path, rec.Code)
		}
	}
	if result, err := h.reconcileProxyEnvironmentOnStartup(); err != nil || result["supported"] != false {
		t.Fatal("Docker reconciled host proxy files")
	}
}

func TestDockerInitialConfigIsReachableAndTunDisabled(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	t.Setenv("APP_NETWORK_SCOPE", "container")
	file := filepath.Join(t.TempDir(), "config.yaml")
	h := newHelper(helperConfig{managedConfig: file})
	if err := h.ensureManagedConfig(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !yamlBoolean(string(raw), "allow-lan", false) || yamlNestedBoolean(string(raw), "tun", "enable", true) || !yamlNestedBoolean(string(raw), "tun", "auto-route", false) {
		t.Fatal("Docker bootstrap network defaults are incorrect")
	}
}

func TestHostBootstrapProxyStaysLocal(t *testing.T) {
	t.Setenv("APP_PLATFORM", "docker")
	t.Setenv("APP_NETWORK_SCOPE", "host")
	file := filepath.Join(t.TempDir(), "config.yaml")
	h := newHelper(helperConfig{managedConfig: file})
	if err := h.ensureManagedConfig(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if yamlBoolean(string(raw), "allow-lan", true) {
		t.Fatal("Host bootstrap exposed proxy on LAN automatically")
	}
}
