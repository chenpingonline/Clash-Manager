package ruleimport

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/klauspost/compress/zstd"
	"gopkg.in/yaml.v3"
)

func TestDetectByContentAndRetainSemantics(t *testing.T) {
	tests := []struct {
		name, content, behavior, format string
		count                           int
	}{
		{"yaml named txt", "payload:\n - '+.example.com'\n - '.sub.example.com'\n - 'exact.example.com'\n", "domain", "yaml", 3},
		{"classical yaml", "payload: [\"DOMAIN-SUFFIX,example.com\", \"IP-CIDR,10.0.0.0/8,no-resolve\"]", "classical", "yaml", 2},
		{"text cidr", "# networks\r\n10.0.0.0/8\r\n2001:db8::/32\n", "ipcidr", "text", 2},
		{"domain text", "\ufeff+.example.com\nexact.example.com\n+.example.com\n", "domain", "text", 2},
		{"classical text", "DOMAIN-SUFFIX,example.com\nIP-CIDR,10.0.0.0/8,no-resolve\n", "classical", "text", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Parse([]byte(tc.content), Options{Name: "Sample", Policy: "DIRECT"}, nil)
			if err != nil || len(r.Errors) > 0 || len(r.Warnings) > 0 {
				t.Fatalf("%+v %v", r, err)
			}
			p := r.Providers[0]
			if p.Behavior != tc.behavior || p.Format != tc.format || p.Count != tc.count {
				t.Fatalf("wrong detection: %+v", p)
			}
			if r.Rules[0] != "RULE-SET,Sample,DIRECT" {
				t.Fatal(r.Rules)
			}
		})
	}
	r, _ := Parse([]byte(tests[0].content), Options{}, nil)
	if r.Providers[0].Sample[1] != ".sub.example.com" {
		t.Fatal("suffix-only wildcard was rewritten")
	}
}
func TestRejectUnsupportedAndMalformedInputs(t *testing.T) {
	for _, content := range []string{"<html>not a rule</html>", "payload: [", "{\"version\":1,\"rules\":null}", "\x00bad"} {
		r, err := Parse([]byte(content), Options{}, nil)
		if err == nil && (r == nil || len(r.Errors) == 0) {
			t.Fatalf("accepted %q: %+v", content, r)
		}
	}
	r, err := Parse([]byte("DOMAIN-SUFFIX,example.com\nHOST-SUFFIX,example.net,Proxy\n"), Options{}, nil)
	if err != nil || len(r.Warnings) != 1 || r.Providers[0].Count != 1 {
		t.Fatalf("unsupported row not surfaced: %+v %v", r, err)
	}
	r, err = Parse([]byte("example.com\n10.0.0.0/8"), Options{}, nil)
	if err != nil || len(r.Errors) != 1 {
		t.Fatalf("mixed list accepted: %+v %v", r, err)
	}
}
func TestINIConversionOrderGroupsAndReplacement(t *testing.T) {
	text := "[custom]\nruleset=Direct,https://example.org/direct.list\nruleset=Proxy,https://example.org/proxy.yaml\nruleset=Direct,[]GEOIP,CN,no-resolve\nruleset=Fallback,[]FINAL\ncustom_proxy_group=Proxy`select`[]DIRECT`(HK|Hong Kong)\ncustom_proxy_group=Direct`select`[]DIRECT\ncustom_proxy_group=Fallback`select`[]Proxy`[]Direct\nenable_rule_generator=true\noverwrite_original_rules=true\nunknown_option=true\n"
	fetch := func(url string) ([]byte, error) {
		if strings.HasSuffix(url, "yaml") {
			return []byte("payload: [\"+.proxy.example.com\"]"), nil
		}
		return []byte("DOMAIN-SUFFIX,direct.example.com"), nil
	}
	r, err := Parse([]byte(text), Options{Name: "ACL"}, fetch)
	if err != nil || len(r.Errors) > 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if len(r.Providers) != 2 || len(r.Groups) != 3 || len(r.Warnings) != 1 || !r.Replace || r.Rules[2] != "GEOIP,CN,Direct,no-resolve" || r.Rules[3] != "MATCH,Fallback" {
		t.Fatalf("wrong conversion: %+v", r)
	}
	raw := []byte("proxies: [{name: HK Node, type: direct}]\nproxy-groups: []\nrules: [\"MATCH,DIRECT\"]\n")
	plan, err := Build(r, Options{}, raw, "A", nil, map[string]string{"override": "dns: {enable: true}\n", "rules": "prepend: [\"DOMAIN,keep.example,DIRECT\"]\n"}, func([]byte) string { return "/mrs" })
	if err != nil || len(plan.Errors) > 0 {
		t.Fatalf("%+v %v", plan, err)
	}
	out, err := configyaml.ApplyProfileExtensionChain(raw, "A", nil, plan.Extensions)
	if err != nil {
		t.Fatal(err)
	}
	var final map[string]any
	_ = yaml.Unmarshal(out, &final)
	rules := final["rules"].([]any)
	if len(rules) != 4 || rules[3] != "MATCH,Fallback" {
		t.Fatal(rules)
	}
	groups := final["proxy-groups"].([]any)
	proxy := groups[0].(map[string]any)
	if proxy["include-all-proxies"] != true || !strings.Contains(proxy["filter"].(string), "HK") {
		t.Fatal(proxy)
	}
	if final["dns"].(map[string]any)["enable"] != true {
		t.Fatal("unrelated override lost")
	}
}
func TestPlanPreservesEnhancementsAndBlocksReferences(t *testing.T) {
	raw := []byte("proxies: []\nproxy-groups: [{name: Proxy, type: select, proxies: [DIRECT]}]\nrules: [\"MATCH,DIRECT\"]\n")
	existing := map[string]string{"rules": "prepend: [\"DOMAIN,old.example,DIRECT\"]\nappend: []\ndelete: []\n", "override": "dns: {enable: true}\n"}
	r, _ := Parse([]byte("example.com"), Options{Name: "New", Policy: "Proxy"}, nil)
	p, err := Build(r, Options{}, raw, "A", nil, existing, func([]byte) string { return "/mrs" })
	if err != nil || len(p.Errors) > 0 {
		t.Fatalf("%+v %v", p, err)
	}
	if !strings.Contains(p.Extensions["rules"], "old.example") || !strings.Contains(p.Extensions["override"], "dns:") {
		t.Fatal(p.Extensions)
	}
	r, _ = Parse([]byte("example.com"), Options{Name: "Missing", Policy: "missing"}, nil)
	p, _ = Build(r, Options{}, raw, "A", nil, existing, func([]byte) string { return "/mrs" })
	if len(p.Errors) == 0 {
		t.Fatal("missing policy allowed")
	}
	r, _ = Parse([]byte("proxy-groups: [{name: A, type: select, proxies: [B]}, {name: B, type: select, proxies: [A]}]\nrules: [\"MATCH,A\"]"), Options{}, nil)
	p, _ = Build(r, Options{}, raw, "A", nil, nil, func([]byte) string { return "/mrs" })
	if len(p.Errors) == 0 {
		t.Fatal("group cycle allowed")
	}
	r, _ = Parse([]byte("example.com"), Options{Name: "New"}, nil)
	p, _ = Build(r, Options{}, raw, "A", map[string]string{"override": "rules: [\"MATCH,DIRECT\"]"}, nil, func([]byte) string { return "/mrs" })
	if len(p.Errors) == 0 {
		t.Fatal("shadowed import allowed")
	}
}
func TestRemoteURLsAndFormatMetadata(t *testing.T) {
	u, err := URL("https://github.com/ACL4SSR/ACL4SSR/blob/master/Clash/BanAD.list")
	if err != nil || u != "https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/BanAD.list" {
		t.Fatal(u, err)
	}
	for _, url := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:pass@example.com/list"} {
		if _, err := URL(url); err == nil {
			t.Fatal("accepted", url)
		}
	}
	r, err := Parse([]byte("payload: ['+.example.com']"), Options{Name: "Remote", URL: "https://example.org/domains.txt"}, nil)
	if err != nil || r.Providers[0].Format != "yaml" || r.Providers[0].Config["type"] != "http" {
		t.Fatal(r, err)
	}
}
func mrsFixture(t *testing.T, behavior byte) []byte {
	t.Helper()
	var b bytes.Buffer
	b.Write([]byte{'M', 'R', 'S', 1, behavior})
	_ = binary.Write(&b, binary.BigEndian, int64(1))
	_ = binary.Write(&b, binary.BigEndian, int64(0))
	b.WriteByte(1)
	if behavior == 0 {
		for _, v := range []int64{1, 2, 1, 6, 1} {
			_ = binary.Write(&b, binary.BigEndian, v)
		}
		b.WriteByte('a')
	} else {
		_ = binary.Write(&b, binary.BigEndian, int64(1))
		b.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 10, 0, 0, 0})
		b.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 10, 255, 255, 255})
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	return encoder.EncodeAll(b.Bytes(), nil)
}
func TestMRSBehaviorCountAndCorruption(t *testing.T) {
	for _, behavior := range []byte{0, 1} {
		r, err := Parse(mrsFixture(t, behavior), Options{Name: "Binary"}, nil)
		if err != nil || len(r.Errors) > 0 || r.Providers[0].Count != 1 || len(r.Providers[0].Asset) == 0 {
			t.Fatal(r, err)
		}
	}
	raw := mrsFixture(t, 0)
	if _, err := Parse(raw[:len(raw)-2], Options{}, nil); err == nil {
		t.Fatal("truncated MRS accepted")
	}
	if _, err := Parse(raw, Options{Behavior: "classical"}, nil); err == nil {
		t.Fatal("MRS classical accepted")
	}
}

func TestFullTextPoliciesAndOriginalLineDiagnostics(t *testing.T) {
	r, err := Parse([]byte("# routing\nDOMAIN,example.com,DIRECT\nMATCH,REJECT\n"), Options{}, nil)
	if err != nil || len(r.Rules) != 2 || len(r.Providers) != 0 || len(r.Errors) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Parse([]byte("# comment\n\nDOMAIN,example.com\nunsupported,value\n"), Options{}, nil)
	if err != nil || len(r.Warnings) != 1 || r.Warnings[0].Line != 4 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Parse([]byte("DOMAIN,example.com,DIRECT\nDOMAIN,missing.example\n"), Options{}, nil)
	if err != nil || len(r.Errors) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if FullRule("IP-CIDR,invalid,DIRECT") == nil {
		t.Fatal("invalid CIDR accepted")
	}
}

func TestACLRemoteLegacyMatchersKeepUpdatesWithExplicitWarnings(t *testing.T) {
	fetch := func(string) ([]byte, error) {
		return []byte("DOMAIN-SUFFIX,example.com\nURL-REGEX,^https://example.com\nUSER-AGENT,Example*"), nil
	}
	r, err := Parse([]byte("[custom]\nruleset=DIRECT,https://example.org/legacy.list\n"), Options{}, fetch)
	if err != nil || len(r.Errors) != 0 || len(r.Warnings) != 2 || len(r.Providers) != 1 || r.Providers[0].Count != 1 || r.Providers[0].Config["type"] != "http" {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Parse([]byte("DOMAIN,example.com\nURL-REGEX,^https://example.com"), Options{URL: "https://example.org/list"}, nil)
	if err != nil || len(r.Errors) != 0 || len(r.Warnings) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestINILongPrefixKeepsProviderReferencesUnique(t *testing.T) {
	r, err := Parse([]byte("[custom]\nruleset=DIRECT,https://example.org/a\nruleset=DIRECT,https://example.org/b\n"), Options{Name: strings.Repeat("名", 100)}, func(string) ([]byte, error) { return []byte("example.com"), nil })
	if err != nil || len(r.Errors) > 0 || len(r.Providers) != 2 || r.Providers[0].Name == r.Providers[1].Name {
		t.Fatalf("%+v %v", r, err)
	}
	for i, p := range r.Providers {
		if r.Rules[i] != "RULE-SET,"+p.Name+",DIRECT" {
			t.Fatal(r.Rules)
		}
	}
}
