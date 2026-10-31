package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
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
