package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestReplaceTopLevelPreservesPositionAndOtherSections(t *testing.T) {
	raw := "# proxy settings\nallow-lan: false\nproxy-groups:\n  - {name: Z, type: select, proxies: [B, A]}\n  - {name: A, type: select, proxies: [Z, DIRECT]}\nrules:\n  - DOMAIN,example.com,Z\n  - MATCH,A\ndns:\n  enable: false\n  nameserver: [1.1.1.1]\nipv6: true\n"
	got := replaceTopLevel(raw, "allow-lan", "allow-lan: true")
	got = replaceTopLevel(got, "dns", "dns:\n  enable: true\n  nameserver: [8.8.8.8]")
	want := strings.Replace(raw, "allow-lan: false", "allow-lan: true", 1)
	want = strings.Replace(want, "enable: false\n  nameserver: [1.1.1.1]", "enable: true\n  nameserver: [8.8.8.8]", 1)
	if got != want {
		t.Fatalf("existing fields moved or unrelated content changed:\n%s", got)
	}
	if next := replaceTopLevel(got, "dns", "dns:\n  enable: true\n  nameserver: [8.8.8.8]"); next != got {
		t.Fatalf("repeated save changed config:\n%s", next)
	}
	if got = replaceTopLevel(got, "dns", ""); got != strings.Replace(want, "dns:\n  enable: true\n  nameserver: [8.8.8.8]\n", "", 1) {
		t.Fatalf("field removal changed other sections:\n%s", got)
	}
	if got = replaceTopLevel(got, "bind-address", "bind-address: '*'"); !strings.HasSuffix(got, "ipv6: true\nbind-address: '*'\n") {
		t.Fatalf("new field was not appended:\n%s", got)
	}
}

func TestNetworkSavesHaveStableFieldAndSequenceOrder(t *testing.T) {
	h := offlineNetworkHelper(t)
	ctx := context.Background()
	raw := "external-controller: 127.0.0.1:9191\nsecret: keep-secret\nproxy-groups:\n  - {name: Z, type: select, proxies: [B, A]}\n  - {name: A, type: select, proxies: [Z, DIRECT]}\nrules:\n  - DOMAIN,example.com,Z\n  - MATCH,A\n"
	input := map[string]any{
		"allowLan": false, "bindAddress": "127.0.0.1",
		"core": map[string]any{"ipv6": false, "unifiedDelay": true},
	}
	want := raw + "allow-lan: false\nbind-address: \"127.0.0.1\"\nipv6: false\nunified-delay: true\n"
	for i := 0; i < 16; i++ {
		// Fresh configs expose random traversal; repeated saves expose relocation.
		source := raw
		if i%2 == 1 {
			source = want
		}
		if err := atomicWrite(h.config.managedConfig, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		prepared, err := h.updateNetwork(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		id := prepared["txId"].(string)
		got, err := os.ReadFile(h.transactions[id].Candidate)
		h.commitConfig(id)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("save %d changed field or sequence order:\n%s", i, got)
		}
	}
}
