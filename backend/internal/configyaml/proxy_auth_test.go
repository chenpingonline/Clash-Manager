package configyaml

import (
	"strings"
	"testing"
)

func TestProxyAuthenticationReadsManualYAML(t *testing.T) {
	raw := []byte("authentication: &accounts ['user:p:a:ss', '用户: 密码 ']\nskip-auth-prefixes: [127.0.0.0/8, '::1/128', 192.168.0.0/16]\nlisteners: [{name: own, type: mixed, users: [{username: other, password: untouched}]}]\n")
	auth, err := ParseProxyAuthentication(raw)
	if err != nil || len(auth.Accounts) != 2 || auth.Accounts[0].Password != "p:a:ss" || auth.Accounts[1].Password != " 密码 " || len(auth.SkipAuthPrefixes) != 3 {
		t.Fatal("manual authentication was not read losslessly", err)
	}
	patch, err := auth.Overrides()
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeOverrides(raw, patch)
	if err != nil || !strings.Contains(string(merged), "untouched") {
		t.Fatal("listener-specific users were modified", err)
	}
	actual, err := ParseProxyAuthentication(merged)
	if err != nil || actual.Accounts[0] != auth.Accounts[0] || actual.Accounts[1] != auth.Accounts[1] {
		t.Fatal("authentication did not round-trip", err)
	}
}

func TestProxyAuthenticationValidatesWithoutExposingCredentials(t *testing.T) {
	for _, auth := range []ProxyAuthentication{
		{Accounts: []ProxyAccount{{"", "sensitive"}}},
		{Accounts: []ProxyAccount{{"user", ""}}},
		{Accounts: []ProxyAccount{{"bad:user", "sensitive"}}},
		{Accounts: []ProxyAccount{{"user", "sensitive\n"}}},
		{Accounts: []ProxyAccount{{"user", "sensitive"}, {"user", "other"}}},
		{SkipAuthPrefixes: []string{"not-a-prefix"}},
	} {
		_, err := auth.Overrides()
		if err == nil || strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "bad:user") {
			t.Fatal("invalid authentication accepted or error exposed credentials")
		}
	}
	for _, source := range []string{"", "authentication: null\n", "authentication: []\nskip-auth-prefixes: []\n"} {
		auth, err := ParseProxyAuthentication([]byte(source))
		if err != nil || auth.Accounts == nil || auth.SkipAuthPrefixes == nil || len(auth.Accounts) != 0 {
			t.Fatal("missing/disabled authentication was not normalized", err)
		}
	}
	for _, source := range []string{"authentication: user:password\n", "authentication: [true]\n", "authentication: [invalid]\n"} {
		if _, err := ParseProxyAuthentication([]byte(source)); err == nil {
			t.Fatal("malformed authentication accepted")
		}
	}
}
