package configyaml

import "testing"

func TestTunDNSDependenciesAcrossConfigEntrypoints(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		err              bool
		issues, warnings int
	}{
		{"tun: {enable: true, dns-hijack: ['any:53']}\ndns: {enable: false}", true, 1, 0},
		{"tun: {enable: false, dns-hijack: ['any:53']}\ndns: {enable: false}", false, 1, 0},
		{"tun: {enable: true, dns-hijack: []}\ndns: {enable: false}", false, 0, 0},
		{"tun: {enable: true, dns-hijack: ['any:53']}\ndns: {enable: true, enhanced-mode: fake-ip}", false, 0, 1},
		{"tun: {enable: true}\ndns: {enable: true, enhanced-mode: fake-ip, fake-ip-range: invalid}", true, 1, 0},
		{"tun: {enable: true}\ndns: {enable: true, enhanced-mode: fake-ip, fake-ip-range: 198.18.0.1/16}", false, 0, 0},
	} {
		s, err := TunDNSRequirements([]byte(tc.raw))
		if (err != nil) != tc.err || len(s.Issues) != tc.issues || len(s.Warnings) != tc.warnings {
			t.Fatal(tc.raw, s, err)
		}
	}
}
