package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirewallRuleOutputBound(t *testing.T) {
	var output firewallRuleOutput
	input := []byte(strings.Repeat("a", maxFirewallRuleBytes+17))
	if n, err := output.Write(input); err != nil || n != len(input) {
		t.Fatal(n, err)
	}
	if n, err := output.Write([]byte("more")); err != nil || n != 4 {
		t.Fatal(n, err)
	}
	var copied firewallRuleOutput
	if _, err := io.Copy(&copied, strings.NewReader(string(input))); err != nil {
		t.Fatal(err)
	}
	if copied.Len() != maxFirewallRuleBytes || !copied.truncated {
		t.Fatal("copy bypassed size limit")
	}
	if output.Len() != maxFirewallRuleBytes || !output.truncated {
		t.Fatal(output.Len(), output.truncated)
	}
}
func TestFirewallRulesReadOnlyFixedCommand(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$*\" = \"--numeric --handle list ruleset\" ] || exit 7\nprintf 'table inet example { # handle 1\\n}\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	h := testHelper(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/network/bypass/rules?command=flush", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "table inet example") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if fileExists(h.config.managedConfig) || fileExists(h.userSettingsPath()) {
		t.Fatal("read created configuration")
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/network/bypass/rules", strings.NewReader("{}")))
	if rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}
func TestFirewallRulesErrorsDoNotReturnPartialOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	h := testHelper(t)
	if _, err := h.firewallRules(context.Background()); err == nil {
		t.Fatal("missing tool accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte("#!/bin/sh\nprintf 'partial-private-rule'\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/network/bypass/rules", nil))
	if rec.Code != 502 || strings.Contains(rec.Body.String(), "partial-private-rule") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.firewallRules(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}
