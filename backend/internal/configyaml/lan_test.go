package configyaml

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnsureLANBindAddress(t *testing.T) {
	loopbacks := []string{"127.0.0.1", "127.2.3.4", "127.1", "127.0.1", "127.16777215", "::1", "[::1]", "0:0:0:0:0:0:0:1", "localhost", "LOCALHOST", " localhost "}
	custom := []string{"192.168.1.2", "0.0.0.0", "*", "::", "[fd00::1]", "", "127.16777216", "127.256.1", "127.0.65536", "127.0.0.256", "127.0.invalid"}
	for _, enabled := range []bool{true, false} {
		for _, bind := range append(loopbacks, custom...) {
			t.Run(fmt.Sprintf("%t/%q", enabled, bind), func(t *testing.T) {
				raw := []byte(fmt.Sprintf("# keep subscription\nallow-lan: %t\nbind-address: %q\nexternal-controller: 127.0.0.1:9090\ndns: {ipv6: false}\nrules: [MATCH,DIRECT]\n", enabled, bind))
				result, err := EnsureLANBindAddress(raw)
				if err != nil {
					t.Fatal(err)
				}
				want := bind
				if enabled {
					for _, loopback := range loopbacks {
						if loopback == bind {
							want = "*"
						}
					}
				}
				if want == bind && string(result) != string(raw) {
					t.Fatalf("unchanged LAN settings rewrote the config: %s", result)
				}
				var got map[string]any
				if err := yaml.Unmarshal(result, &got); err != nil {
					t.Fatal(err)
				}
				if got["bind-address"] != want || got["allow-lan"] != enabled || got["external-controller"] != "127.0.0.1:9090" || got["dns"].(map[string]any)["ipv6"] != false || !strings.Contains(string(result), "# keep subscription") {
					t.Fatalf("unexpected normalized config: %s", result)
				}
			})
		}
	}
}

func TestEnsureLANDefaultBindingAndFlowYAML(t *testing.T) {
	raw := []byte("allow-lan: true\nipv6: true\n")
	result, err := EnsureLANBindAddress(raw)
	if err != nil || string(result) != string(raw) {
		t.Fatalf("missing bind-address should use the core default: %s %v", result, err)
	}
	result, err = EnsureLANBindAddress([]byte(`{allow-lan: true, bind-address: "127.0.0.1", ipv6: false}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = yaml.Unmarshal(result, &got); err != nil || got["bind-address"] != "*" || got["ipv6"] != false {
		t.Fatalf("flow YAML normalization failed: %s %v", result, err)
	}
	if _, err = EnsureLANBindAddress([]byte("allow-lan: [")); err == nil {
		t.Fatal("invalid YAML accepted")
	}
}
