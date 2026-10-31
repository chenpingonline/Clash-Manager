package configyaml

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func listenerFixture() IndependentListener {
	return IndependentListener{ID: "test-id", Name: "US entry", Type: "mixed", Listen: "127.0.0.1", Port: 1082, Proxy: "US", Enabled: true, Users: []ProxyAccount{}}
}
func TestIndependentListenersSubscriptionMergeAndRecovery(t *testing.T) {
	entry := listenerFixture()
	source := "# subscription\nproxies: [{name: US, type: socks5, server: old.example, port: 1080}]\nlisteners:\n  - {name: original, type: http, port: 1083}\nrules: [MATCH,DIRECT]\n"
	merged, missing, err := MergeIndependentListeners([]byte(source), []IndependentListener{entry}, nil)
	if err != nil || len(missing) != 0 {
		t.Fatal(err, missing)
	}
	for _, want := range []string{"# subscription", "original", listenerMarker + entry.ID, "users: []", "proxy: US", "udp: true"} {
		if !strings.Contains(string(merged), want) {
			t.Fatalf("missing %s", want)
		}
	}
	again, _, err := MergeIndependentListeners(merged, []IndependentListener{entry}, []IndependentListener{entry})
	if err != nil || strings.Count(string(again), "name: US entry") != 1 {
		t.Fatal("not idempotent", err, string(again))
	}
	// A fresh subscription resolves the same target name to new connection details.
	updated := strings.Replace(source, "old.example", "new.example", 1)
	merged, _, err = MergeIndependentListeners([]byte(updated), []IndependentListener{entry}, []IndependentListener{entry})
	if err != nil || !strings.Contains(string(merged), "new.example") || strings.Contains(string(merged), "old.example") {
		t.Fatal(err)
	}
	missingSource := strings.Replace(updated, "name: US,", "name: renamed,", 1)
	absent, missing, err := MergeIndependentListeners([]byte(missingSource), []IndependentListener{entry}, []IndependentListener{entry})
	if err != nil || len(missing) != 1 || missing[0] != entry.ID || strings.Contains(string(absent), "name: US entry") || !strings.Contains(string(absent), "original") {
		t.Fatal("missing target not isolated", err, string(absent), missing)
	}
	_, missing, err = MergeIndependentListeners([]byte(updated), []IndependentListener{entry}, []IndependentListener{entry})
	if err != nil || len(missing) > 0 {
		t.Fatal("target did not recover", err)
	}
	// Removing managed entries must leave subscription entries untouched.
	removed, _, err := MergeIndependentListeners(merged, []IndependentListener{}, []IndependentListener{entry})
	if err != nil || strings.Contains(string(removed), "US entry") || !strings.Contains(string(removed), "original") {
		t.Fatal(err, string(removed))
	}
}
func TestIndependentListenerCollisionAndCredentials(t *testing.T) {
	entry := listenerFixture()
	if _, _, err := MergeIndependentListeners([]byte("listeners: [{name: US entry, type: http, port: 9000}]"), []IndependentListener{entry}, nil); err == nil {
		t.Fatal("overwrote subscription listener")
	}
	entry.Users = []ProxyAccount{{Username: "user", Password: "p:a:ss"}}
	raw, _, err := MergeIndependentListeners([]byte("proxy-groups: [{name: US, type: select, proxies: [DIRECT]}]"), []IndependentListener{entry}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Listeners []struct {
			Users []ProxyAccount `yaml:"users"`
		} `yaml:"listeners"`
	}
	if err = yaml.Unmarshal(raw, &config); err != nil || config.Listeners[0].Users[0].Password != "p:a:ss" {
		t.Fatal("lost credentials", err)
	}
}
func TestIndependentListenerValidation(t *testing.T) {
	for _, mutate := range []func(*IndependentListener){func(e *IndependentListener) { e.ID = "../bad" }, func(e *IndependentListener) { e.Name = "" }, func(e *IndependentListener) { e.Type = "exec" }, func(e *IndependentListener) { e.Listen = "example.com" }, func(e *IndependentListener) { e.Port = 65536 }, func(e *IndependentListener) { e.Proxy = "" }, func(e *IndependentListener) { e.Users = []ProxyAccount{{Username: "x", Password: ""}} }} {
		entry := listenerFixture()
		mutate(&entry)
		if ValidateIndependentListeners([]IndependentListener{entry}) == nil {
			t.Fatal("invalid entry accepted", entry)
		}
	}
	entry := listenerFixture()
	if ValidateIndependentListeners([]IndependentListener{entry, entry}) == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestIndependentListenersPreserveSubscriptionAnchors(t *testing.T) {
	for _, source := range []string{
		"listeners: &inbounds [{name: original, type: http, port: 1083}]\nother: *inbounds\n",
		"original: &inbounds [{name: original, type: http, port: 1083}]\nlisteners: *inbounds\n",
	} {
		entry := listenerFixture()
		entry.Proxy = "DIRECT"
		merged, _, err := MergeIndependentListeners([]byte(source), []IndependentListener{entry}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err = yaml.Unmarshal(merged, &config); err != nil {
			t.Fatal("broke an existing subscription alias", err, string(merged))
		}
		if len(config["listeners"].([]any)) != 2 {
			t.Fatal("lost listener entries")
		}
	}
}
