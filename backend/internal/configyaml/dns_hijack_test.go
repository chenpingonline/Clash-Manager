package configyaml

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNormalizeDNSHijackEndpoints(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"114.114.114.114", "114.114.114.114:53"},
		{"2001:4860:4860::8888", "[2001:4860:4860::8888]:53"},
		{"[2001:db8::1]", "[2001:db8::1]:53"},
		{"udp://223.5.5.5", "udp://223.5.5.5:53"},
		{"tcp://2001:db8::1", "tcp://[2001:db8::1]:53"},
		{"any:53", "any:53"},
		{"tcp://any:5353", "tcp://any:5353"},
		{"udp://0.0.0.0:53", "udp://0.0.0.0:53"},
		{"114.114.114.114:1053", "114.114.114.114:1053"},
		{"tcp://[2001:db8::1]:65535", "tcp://[2001:db8::1]:65535"},
		{" 223.5.5.5 ", "223.5.5.5:53"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeDNSHijackEndpoint(tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("got=%q want=%q err=%v", got, tc.want, err)
			}
		})
	}
	for _, input := range []string{"", "any", "invalid", "example.com:53", "999.1.1.1", "1.1.1.1:", "1.1.1.1:0", "any:65536", "1.1.1.1:-1", "1.1.1.1:dns", "tcp://", "tcp://1.1.1.1:53/path", "https://1.1.1.1:53", "[invalid]:53", "1.1.1.1:53:53"} {
		t.Run("invalid "+input, func(t *testing.T) {
			if _, err := normalizeDNSHijackEndpoint(input); err == nil {
				t.Fatalf("accepted %q", input)
			}
		})
	}
}

func TestNormalizeTunDNSHijackPreservesConfigAndComments(t *testing.T) {
	raw := []byte("# subscription\ndns: {enable: true, nameserver: [223.5.5.5]}\ntun:\n  enable: false\n  device: keep-me\n  dns-hijack:\n    - 114.114.114.114 # primary\n    - tcp://any:5353 # custom port\nproxy-groups:\n  - {name: Z, type: select, proxies: [DIRECT]}\n  - {name: A, type: select, proxies: [Z]}\nrules: ['DOMAIN,example.com,Z', 'MATCH,A']\n")
	got, err := NormalizeTunDNSHijack(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"# subscription", "114.114.114.114:53 # primary", "tcp://any:5353 # custom port", "device: keep-me"} {
		if !strings.Contains(string(got), text) {
			t.Fatalf("lost %q in %s", text, got)
		}
	}
	var before, after map[string]any
	if err := yaml.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(got, &after); err != nil {
		t.Fatal(err)
	}
	before["tun"].(map[string]any)["dns-hijack"] = []any{"114.114.114.114:53", "tcp://any:5353"}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("unrelated config changed: %s", got)
	}
	if groups := ProxyGroupOrder(string(got)); !reflect.DeepEqual(groups, []string{"Z", "A"}) {
		t.Fatalf("group order=%v", groups)
	}
	again, err := NormalizeTunDNSHijack(got)
	if err != nil || !bytes.Equal(got, again) {
		t.Fatalf("not idempotent: %s err=%v", again, err)
	}
}

func TestNormalizeTunDNSHijackAliasesAndMerges(t *testing.T) {
	for _, raw := range []string{
		"targets: &targets [114.114.114.114]\ntun: {dns-hijack: *targets}\n",
		"defaults: &defaults {enable: false, dns-hijack: [114.114.114.114]}\ntun: *defaults\n",
		"defaults: &defaults {dns-hijack: [114.114.114.114]}\ntun: {<<: *defaults, enable: false}\n",
		"defaults: &defaults {tun: {dns-hijack: [114.114.114.114]}}\n<<: *defaults\n",
	} {
		got, err := NormalizeTunDNSHijack([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := yaml.Unmarshal(got, &document); err != nil {
			t.Fatal(err)
		}
		if targets := document["tun"].(map[string]any)["dns-hijack"]; !reflect.DeepEqual(targets, []any{"114.114.114.114:53"}) {
			t.Fatalf("targets=%v in %s", targets, got)
		}
	}
}

func TestNormalizeTunDNSHijackReportsMalformedConfig(t *testing.T) {
	for _, tc := range []struct{ raw, message string }{
		{"tun: {dns-hijack: true}\n", "必须是地址字符串列表"},
		{"tun: {dns-hijack: '1.1.1.1'}\n", "必须是地址字符串列表"},
		{"tun: {dns-hijack: [any:53, false]}\n", "第 2 项必须是地址字符串"},
		{"tun: {dns-hijack: [any:53, invalid]}\n", "第 2 项格式无效"},
		{"dns: {dns-hijack: [1.1.1.1]}\n", "应放在 tun 下"},
	} {
		if _, err := NormalizeTunDNSHijack([]byte(tc.raw)); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("raw=%s err=%v want=%s", tc.raw, err, tc.message)
		}
	}
	for _, raw := range []string{"mixed-port: 7890\n", "tun: {dns-hijack: []}\n", "tun: {dns-hijack: null}\n", "tun: {dns-hijack: [any:53, 'tcp://any:53']}\n"} {
		got, err := NormalizeTunDNSHijack([]byte(raw))
		if err != nil || string(got) != raw {
			t.Fatalf("valid config changed: got=%s err=%v", got, err)
		}
	}
}
