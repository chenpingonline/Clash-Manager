package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
)

func independentInput(t *testing.T, entries []configyaml.IndependentListener) map[string]any {
	t.Helper()
	data, _ := json.Marshal(entries)
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"independentListeners": value}
}
func TestIndependentListenerPreferencesTransaction(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	entry := configyaml.IndependentListener{ID: "retained", Name: "Local entry", Type: "mixed", Listen: "127.0.0.1", Port: 40182, Proxy: "DIRECT", Enabled: true, Users: []configyaml.ProxyAccount{}}
	prepared, err := h.updateNetwork(ctx, independentInput(t, []configyaml.IndependentListener{entry}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
		t.Fatal("persisted before activation")
	}
	id := prepared["txId"].(string)
	if _, err = h.activateConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = h.commitConfig(id); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(h.userSettingsPath())
	restarted := &helper{config: h.config}
	composed, err := restarted.composeUserSettings(map[string]any{"content": "proxies: []\nlisteners: [{name: subscription, type: http, port: 40183}]\nrules: [MATCH,DIRECT]\n"})
	if err != nil {
		t.Fatal(err)
	}
	content := composed["content"].(string)
	if !strings.Contains(content, "Local entry") || !strings.Contains(content, "subscription") || strings.Contains(content, configyaml.IndependentListenersKey) {
		t.Fatal("subscription preferences lost/leaked", content)
	}
	status, err := restarted.networkStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	saved := status["settings"].(map[string]any)["independentListeners"].(map[string]any)["entries"].([]configyaml.IndependentListener)
	if len(saved) != 1 || saved[0].ID != entry.ID {
		t.Fatal("status lost saved entry")
	}
	prepared, err = h.updateNetwork(ctx, independentInput(t, []configyaml.IndependentListener{}))
	if err != nil {
		t.Fatal(err)
	}
	id = prepared["txId"].(string)
	if _, err = h.activateConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = h.rollbackConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(h.userSettingsPath())
	if string(restored) != string(original) {
		t.Fatal("rollback lost preferences")
	}
}
func TestIndependentListenerPreflightAndRejectedInput(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	entry := configyaml.IndependentListener{ID: "conflict", Name: "Port conflict", Type: "mixed", Listen: "127.0.0.1", Port: 7890, Proxy: "DIRECT", Enabled: true}
	if _, err := h.updateNetwork(ctx, independentInput(t, []configyaml.IndependentListener{entry})); err == nil || !strings.Contains(err.Error(), "端口冲突") {
		t.Fatal("top-level conflict accepted", err)
	}
	if _, err := h.updateNetwork(ctx, map[string]any{"independentListeners": nil}); err == nil {
		t.Fatal("null accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entry.Port = listener.Addr().(*net.TCPAddr).Port
	if _, err = h.updateNetwork(ctx, independentInput(t, []configyaml.IndependentListener{entry})); err == nil {
		t.Fatal("occupied port accepted")
	}
	if len(h.transactions) != 0 {
		t.Fatal("failed validation created transaction")
	}
}

func setListenerCurrentProfile(t *testing.T, h *helper, id string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"current": id, "items": []any{}})
	if err := os.WriteFile(filepath.Join(h.config.etcDir, "profiles.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
}
func scopedListenerEntry(id string, port int) configyaml.IndependentListener {
	return configyaml.IndependentListener{ID: id, Name: id, Type: "mixed", Listen: "127.0.0.1", Port: port, Proxy: "DIRECT", Enabled: true}
}
func saveScopedListeners(t *testing.T, h *helper, scope string, enabled bool, entries ...configyaml.IndependentListener) {
	t.Helper()
	if entries == nil {
		entries = []configyaml.IndependentListener{}
	}
	_, err := h.saveInactiveIndependentListeners(map[string]any{"scope": scope, "content": "mixed-port: 7890\nproxies: []\n", "enabled": enabled, "entries": entries})
	if err != nil {
		t.Fatal(err)
	}
}
func TestIndependentListenersScopeMigrationAndComposition(t *testing.T) {
	h := offlineNetworkHelper(t)
	setListenerCurrentProfile(t, h, "a")
	legacy := scopedListenerEntry("legacy-a", 41082)
	body, _ := json.Marshal(map[string]any{configyaml.IndependentListenersKey: []configyaml.IndependentListener{legacy}, "ipv6": true})
	if err := atomicWrite(h.userSettingsPath(), body, 0600); err != nil {
		t.Fatal(err)
	}
	// First access can be a switch to B. Legacy preferences must bind to old A.
	compose := func(scope string) string {
		t.Helper()
		result, err := h.composeUserSettings(map[string]any{"listenerScope": scope, "content": "proxies: []\nlisteners: [{name: subscription, type: http, port: 41083}]\n"})
		if err != nil {
			t.Fatal(err)
		}
		return result["content"].(string)
	}
	if raw := compose("profile:b"); strings.Contains(raw, "legacy-a") || !strings.Contains(raw, "subscription") || strings.Contains(raw, "manager-independent") {
		t.Fatal(raw)
	}
	migrated, _ := os.ReadFile(h.userSettingsPath())
	if strings.Contains(string(migrated), configyaml.IndependentListenersKey) || !strings.Contains(string(migrated), "profile:a") {
		t.Fatal("legacy binding not persisted", string(migrated))
	}
	if raw := compose("profile:a"); !strings.Contains(raw, "legacy-a") {
		t.Fatal(raw)
	}
	b := scopedListenerEntry("b-only", 41084)
	saveScopedListeners(t, h, "profile:b", true, b)
	if raw := compose("profile:b"); strings.Contains(raw, "legacy-a") || !strings.Contains(raw, "b-only") {
		t.Fatal(raw)
	}
	saveScopedListeners(t, h, "profile:a", false, legacy)
	if raw := compose("profile:a"); strings.Contains(raw, "legacy-a") || !strings.Contains(raw, "subscription") {
		t.Fatal("master switch removed subscription", raw)
	}
	state, err := h.readIndependentListeners(map[string]any{"scope": "profile:a", "content": "proxies: []\n"})
	if err != nil || state["enabled"] != false || len(state["entries"].([]configyaml.IndependentListener)) != 1 {
		t.Fatal(state, err)
	}
	saveScopedListeners(t, h, "profile:a", true, legacy)
	if raw := compose("profile:a"); !strings.Contains(raw, "legacy-a") {
		t.Fatal("reenable lost entry", raw)
	}
	restarted := &helper{config: h.config}
	state, err = restarted.readIndependentListeners(map[string]any{"scope": "profile:b", "content": "proxies: []\n"})
	if err != nil || state["entries"].([]configyaml.IndependentListener)[0].ID != "b-only" {
		t.Fatal(state, err)
	}
	if _, err := h.deleteIndependentListeners(map[string]any{"scope": "profile:b"}); err != nil {
		t.Fatal(err)
	}
	if raw := compose("profile:b"); strings.Contains(raw, "b-only") {
		t.Fatal("deleted preferences survived", raw)
	}
}
func TestInactiveListenerSaveDoesNotTouchRuntimeOrProbeLivePorts(t *testing.T) {
	h := offlineNetworkHelper(t)
	setListenerCurrentProfile(t, h, "a")
	before, _ := os.ReadFile(h.config.managedConfig)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entry := scopedListenerEntry("inactive", listener.Addr().(*net.TCPAddr).Port)
	saveScopedListeners(t, h, "profile:b", true, entry)
	after, _ := os.ReadFile(h.config.managedConfig)
	if string(before) != string(after) || len(h.transactions) > 0 {
		t.Fatal("inactive save changed runtime")
	}
	saved, _ := os.ReadFile(h.userSettingsPath())
	entry.Port = 7890
	if _, err := h.saveInactiveIndependentListeners(map[string]any{"scope": "profile:b", "content": "mixed-port: 7890\n", "enabled": true, "entries": []configyaml.IndependentListener{entry}}); err == nil {
		t.Fatal("configured port conflict accepted")
	}
	after, _ = os.ReadFile(h.userSettingsPath())
	if string(saved) != string(after) {
		t.Fatal("failed save mutated preferences")
	}
}
func TestIndependentMasterSwitchRollbackAndPendingMigration(t *testing.T) {
	h := offlineNetworkHelper(t)
	entry := scopedListenerEntry("runtime-entry", 41089)
	ctx := context.Background()
	prepared, err := h.updateNetwork(ctx, independentInput(t, []configyaml.IndependentListener{entry}))
	if err != nil {
		t.Fatal(err)
	}
	id := prepared["txId"].(string)
	if _, err = h.activateConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = h.commitConfig(id); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(h.userSettingsPath())
	input := independentInput(t, []configyaml.IndependentListener{entry})
	input["independentListenersEnabled"] = false
	prepared, err = h.updateNetwork(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prepared["effectiveContent"].(string), "runtime-entry") {
		t.Fatal("disabled listener still generated")
	}
	id = prepared["txId"].(string)
	if _, err = h.activateConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = h.rollbackConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(h.userSettingsPath())
	if string(original) != string(restored) {
		t.Fatal("master rollback lost intent")
	}
	// An upgrade without a known current profile must retain unassigned entries.
	body, _ := json.Marshal(map[string]any{configyaml.IndependentListenersKey: []configyaml.IndependentListener{entry}})
	_ = atomicWrite(h.userSettingsPath(), body, 0600)
	result, err := h.readIndependentListeners(map[string]any{"scope": "profile:b", "content": "proxies: []\n"})
	if err != nil || len(result["entries"].([]configyaml.IndependentListener)) != 0 || len(result["unassigned"].([]configyaml.IndependentListener)) != 1 {
		t.Fatal(result, err)
	}
	result, err = h.composeUserSettings(map[string]any{"listenerScope": "profile:b", "content": "proxies: []\n"})
	if err != nil || strings.Contains(result["content"].(string), entry.Name) {
		t.Fatal("pending entries leaked", result, err)
	}
}

func TestIndependentUDPDisabledDoesNotReserveUDPPort(t *testing.T) {
	h := offlineNetworkHelper(t)
	socket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	entry := scopedListenerEntry("tcp-only", socket.LocalAddr().(*net.UDPAddr).Port)
	udp := false
	entry.UDP = &udp
	input := independentInput(t, []configyaml.IndependentListener{entry})
	prepared, err := h.updateNetwork(context.Background(), input)
	if err != nil {
		t.Fatal("UDP disabled still probes UDP", err)
	}
	if _, err = h.rollbackConfig(context.Background(), prepared["txId"].(string)); err != nil {
		t.Fatal(err)
	}
	udp = true
	if _, err = h.updateNetwork(context.Background(), independentInput(t, []configyaml.IndependentListener{entry})); err == nil {
		t.Fatal("UDP enabled did not check occupied port")
	}
}
