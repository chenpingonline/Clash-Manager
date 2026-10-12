package main

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
)

func authInput(t *testing.T, auth configyaml.ProxyAuthentication) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(auth)
	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"proxyAuthentication": input}
}

func TestProxyAuthenticationManualReadAndUnrelatedNetworkSave(t *testing.T) {
	h := offlineNetworkHelper(t)
	raw, _ := os.ReadFile(h.config.managedConfig)
	raw = append(raw, []byte("authentication: ['manual:p:a:ss', 'second:other']\nskip-auth-prefixes: [192.168.0.0/16]\n")...)
	if err := atomicWrite(h.config.managedConfig, raw, 0600); err != nil {
		t.Fatal(err)
	}
	status, err := h.networkStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	auth := status["settings"].(map[string]any)["proxyAuthentication"].(configyaml.ProxyAuthentication)
	if len(auth.Accounts) != 2 || auth.Accounts[0].Password != "p:a:ss" {
		t.Fatal("manual accounts missing")
	}
	prepared, err := h.updateNetwork(context.Background(), map[string]any{"core": map[string]any{"ipv6": false}})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := configyaml.ParseProxyAuthentication([]byte(prepared["effectiveContent"].(string)))
	if err != nil || !reflect.DeepEqual(actual, auth) {
		t.Fatal("unrelated network save changed manual authentication", err)
	}
	composed, err := h.composeUserSettings(map[string]any{"content": string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	actual, _ = configyaml.ParseProxyAuthentication([]byte(composed["content"].(string)))
	if !reflect.DeepEqual(actual, auth) {
		t.Fatal("composition changed manual authentication")
	}
}

func TestProxyAuthenticationSaveDisableAndRollback(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	auth := configyaml.ProxyAuthentication{Accounts: []configyaml.ProxyAccount{{Username: "user", Password: "p:a:ss"}, {Username: "second", Password: "other"}}, SkipAuthPrefixes: []string{"127.0.0.0/8", "::1/128"}}
	prepared, err := h.updateNetwork(ctx, authInput(t, auth))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
		t.Fatal("credentials persisted before validation/activation")
	}
	id := prepared["txId"].(string)
	if _, err := h.activateConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := h.commitConfig(id); err != nil {
		t.Fatal(err)
	}
	preferences, _ := os.ReadFile(h.userSettingsPath())
	source := "authentication: ['subscription:original']\nskip-auth-prefixes: []\nrules: []\n"
	restarted := &helper{config: h.config}
	composed, err := restarted.composeUserSettings(map[string]any{"content": source})
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := configyaml.ParseProxyAuthentication([]byte(composed["content"].(string)))
	if !reflect.DeepEqual(actual, auth) {
		t.Fatal("restart/subscription update lost GUI credentials")
	}
	prepared, err = h.updateNetwork(ctx, authInput(t, configyaml.ProxyAuthentication{Accounts: []configyaml.ProxyAccount{}, SkipAuthPrefixes: auth.SkipAuthPrefixes}))
	if err != nil {
		t.Fatal(err)
	}
	id = prepared["txId"].(string)
	if _, err := h.activateConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	composed, err = h.composeUserSettings(map[string]any{"content": source})
	if err != nil {
		t.Fatal(err)
	}
	actual, _ = configyaml.ParseProxyAuthentication([]byte(composed["content"].(string)))
	if len(actual.Accounts) != 0 || !reflect.DeepEqual(actual.SkipAuthPrefixes, auth.SkipAuthPrefixes) {
		t.Fatal("disabled auth resurrected subscription credentials")
	}
	if _, err := h.rollbackConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(h.userSettingsPath())
	if string(restored) != string(preferences) {
		t.Fatal("rollback did not restore auth preferences")
	}
}

func TestManualAuthEditUpdatesExistingGUIOverridesTransactionally(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	if err := atomicWrite(h.userSettingsPath(), []byte(`{"authentication":["old:original"],"skip-auth-prefixes":["127.0.0.0/8"],"ipv6":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"authentication: ['new:p:a:ss']\nskip-auth-prefixes: ['::1/128']\nrules: []\n", "rules: []\n"} {
		prepared, err := h.prepareStartupConfig(ctx, content, true)
		if err != nil {
			t.Fatal(err)
		}
		id := prepared["txId"].(string)
		if _, err := h.activateConfig(ctx, id); err != nil {
			t.Fatal(err)
		}
		composed, err := h.composeUserSettings(map[string]any{"content": "authentication: ['subscription:stale']\nskip-auth-prefixes: [192.168.0.0/16]\n"})
		if err != nil {
			t.Fatal(err)
		}
		want, _ := configyaml.ParseProxyAuthentication([]byte(content))
		got, _ := configyaml.ParseProxyAuthentication([]byte(composed["content"].(string)))
		if !reflect.DeepEqual(got, want) {
			t.Fatal("manual edit was replaced by stale GUI preferences")
		}
		if _, err := h.rollbackConfig(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInvalidProxyAuthenticationDoesNotCreateTransaction(t *testing.T) {
	h := offlineNetworkHelper(t)
	for _, input := range []any{nil, "invalid", map[string]any{"accounts": []any{}}, map[string]any{"accounts": []any{map[string]any{"username": "", "password": "test-only"}}, "skipAuthPrefixes": []any{}}} {
		if _, err := h.updateNetwork(context.Background(), map[string]any{"proxyAuthentication": input}); err == nil {
			t.Fatal("invalid auth accepted")
		}
	}
	if len(h.transactions) != 0 {
		t.Fatal("invalid credentials created a transaction")
	}
	if _, err := os.Stat(h.userSettingsPath()); !os.IsNotExist(err) {
		t.Fatal("invalid credentials persisted")
	}
}
