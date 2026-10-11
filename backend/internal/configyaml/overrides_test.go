package configyaml

import (
	"gopkg.in/yaml.v3"
	"reflect"
	"strings"
	"testing"
)

func TestUserOverridesOnlyReplaceExplicitSettings(t *testing.T) {
	raw := []byte("# subscription\nipv6: true\nunified-delay: false\ngeo-auto-update: false\ntun: {enable: true, mtu: 1400, stack: system}\nproxies: []\nrules:\n  - MATCH,DIRECT\n")
	out, err := MergeOverrides(raw, map[string]any{"ipv6": false, "geo-auto-update": true, "tun": map[string]any{"enable": false, "route-exclude-address": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	tun := got["tun"].(map[string]any)
	if got["ipv6"] != false || got["unified-delay"] != false || got["geo-auto-update"] != true || tun["enable"] != false || tun["mtu"] != 1400 || tun["stack"] != "system" {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(string(out), "# subscription") || len(got["rules"].([]any)) != 1 {
		t.Fatal("lost subscription content")
	}
}

func TestOverridesKeepExistingPositionsAndSequenceOrder(t *testing.T) {
	raw := []byte("secret: keep\ntun: {stack: system, mtu: 1500, enable: false, dns-hijack: [udp://0.0.0.0:53, tcp://0.0.0.0:53]}\nproxy-groups:\n  - {name: Z, type: select, proxies: [B, A]}\n  - {name: A, type: select, proxies: [Z, DIRECT]}\nrules: ['DOMAIN,example.com,Z', 'MATCH,A']\nipv6: true\n")
	patch := map[string]any{
		"ipv6": false, "bind-address": "*", "allow-lan": true,
		"tun": map[string]any{"enable": true, "mtu": nil, "auto-route": true, "auto-detect-interface": true},
	}
	var before map[string]any
	if err := yaml.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	var first []byte
	for i := 0; i < 16; i++ {
		out, err := MergeOverrides(raw, patch)
		if err != nil {
			t.Fatal(err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		keys := func(node *yaml.Node) []string {
			var result []string
			for j := 0; j < len(node.Content); j += 2 {
				result = append(result, node.Content[j].Value)
			}
			return result
		}
		root := doc.Content[0]
		if got := keys(root); !reflect.DeepEqual(got, []string{"secret", "tun", "proxy-groups", "rules", "ipv6", "allow-lan", "bind-address"}) {
			t.Fatalf("top-level fields moved: %v", got)
		}
		if got := keys(root.Content[3]); !reflect.DeepEqual(got, []string{"stack", "enable", "dns-hijack", "auto-detect-interface", "auto-route"}) {
			t.Fatalf("TUN fields moved: %v", got)
		}
		var after map[string]any
		if err := yaml.Unmarshal(out, &after); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"rules", "proxy-groups"} {
			if !reflect.DeepEqual(before[key], after[key]) {
				t.Fatalf("%s sequence changed", key)
			}
		}
		if after["ipv6"] != false || after["tun"].(map[string]any)["enable"] != true {
			t.Fatal("values were not updated")
		}
		if !reflect.DeepEqual(before["tun"].(map[string]any)["dns-hijack"], after["tun"].(map[string]any)["dns-hijack"]) {
			t.Fatal("TUN sequence changed")
		}
		if i == 0 {
			first = out
		} else if string(out) != string(first) {
			t.Fatal("repeated composition changed ordering")
		}
		raw = out
	}
}
func TestNoOverridesPreservesOriginalBytes(t *testing.T) {
	raw := []byte("tun: { enable: true }\n")
	out, err := MergeOverrides(raw, nil)
	if err != nil || string(out) != string(raw) {
		t.Fatalf("%s %v", out, err)
	}
}

func TestOverridePreservesReferencedAnchor(t *testing.T) {
	out, err := MergeOverrides([]byte("tun: &tun {enable: true, mtu: 1400}\nother: *tun\n"), map[string]any{"tun": map[string]any{"enable": false}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	if got["tun"].(map[string]any)["mtu"] != 1400 {
		t.Fatal("lost untouched field")
	}
}
