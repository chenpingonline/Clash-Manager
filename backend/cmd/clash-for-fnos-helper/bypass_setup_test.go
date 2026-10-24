package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func testBypassPlan(mode string) *bypassState {
	return &bypassState{Schema: 1, Token: strings.Repeat("a", 32), Mode: mode, Interface: bypassInterface{"eth0", "192.168.1.2", "192.168.1.0/24"}, Port: 7896, PreviousForward: "0", CorePID: 123}
}
func TestBypassRuleScope(t *testing.T) {
	for _, mode := range []string{"redir", "tproxy"} {
		r := bypassRules(testBypassPlan(mode))
		for _, required := range []string{`iifname "eth0" ip saddr 192.168.1.0/24`, `fib daddr type local return`, `th dport 53 return`, `meta mark != 0 return`} {
			if !strings.Contains(r, required) {
				t.Fatalf("missing %s: %s", required, r)
			}
		}
		if strings.Contains(r, "flush") || strings.Contains(r, "postrouting") || strings.Contains(r, "output") {
			t.Fatal("broad mutation", r)
		}
		if mode == "tproxy" && !strings.Contains(r, "tproxy to :7896") {
			t.Fatal(r)
		}
	}
	for _, i := range []bypassInterface{{"eth0;bad", "192.168.1.2", "192.168.1.0/24"}, {"docker0", "172.17.0.1", "172.17.0.0/16"}, {"eth0", "8.8.8.8", "0.0.0.0/0"}, {"eth0", "10.1.2.3", "10.0.0.0/7"}, {"eth0", "192.168.2.2", "192.168.1.0/24"}} {
		if validBypassInterface(i) {
			t.Fatalf("unsafe interface: %+v", i)
		}
	}
}
func TestBypassTransactionsRollback(t *testing.T) {
	for _, failure := range []string{"route", "rule", "forward", "table", ""} {
		t.Run(failure, func(t *testing.T) {
			h := testHelper(t)
			ctx := context.Background()
			s := testBypassPlan("tproxy")
			route, rule, table := false, false, false
			forward := "0"
			h.bypassSystem = &bypassSystem{
				readForward: func() (string, error) { return forward, nil }, writeForward: func(v string) error {
					if failure == "forward" && v == "1" {
						return errors.New("injected forwarding failure")
					}
					forward = v
					return nil
				},
				run: func(_ context.Context, input, name string, args ...string) ([]byte, error) {
					command := name + " " + strings.Join(args, " ")
					switch command {
					case "nft -j list tables":
						if table {
							return []byte(`{"nftables":[{"table":{"family":"ip","name":"clash_manager_bypass","comment":"clash-manager:` + s.Token + `"}}]}`), nil
						}
						return []byte(`{"nftables":[]}`), nil
					case "ip -4 route add local 0.0.0.0/0 dev lo table 18462":
						if failure == "route" {
							return nil, errors.New("route failure")
						}
						route = true
					case "ip -4 rule add priority 18462 fwmark 0x434d/0xffff lookup 18462":
						if failure == "rule" {
							return nil, errors.New("rule failure")
						}
						rule = true
					case "nft -f -":
						if failure == "table" {
							return nil, errors.New("table failure")
						}
						table = true
					case "nft delete table ip clash_manager_bypass":
						table = false
					case "ip -4 rule show":
						if rule {
							return []byte("18462: from all fwmark 0x434d/0xffff lookup 18462\n"), nil
						}
					case "ip -4 route show table 18462":
						if route {
							return []byte("local default dev lo scope host\n"), nil
						}
					case "ip -4 rule del priority 18462 fwmark 0x434d/0xffff lookup 18462":
						rule = false
					case "ip -4 route del local 0.0.0.0/0 dev lo table 18462":
						route = false
					default:
						t.Fatalf("unexpected system call %s", command)
					}
					return []byte{}, nil
				},
			}
			_, err := h.installBypassPlan(ctx, s)
			if failure == "" {
				if err != nil || !table || !route || !rule || forward != "1" {
					t.Fatal(err, table, route, rule, forward)
				}
				if err = h.cleanupBypass(ctx, 0); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("missing failure")
			}
			if table || rule || route || forward != "0" || fileExists(h.bypassJournalPath()) {
				t.Fatal("rollback left owned mutations", table, rule, route, forward)
			}
		})
	}
}
func TestBypassCleanupRefusesForeignTable(t *testing.T) {
	h := testHelper(t)
	s := testBypassPlan("redir")
	if err := h.saveBypassState(s); err != nil {
		t.Fatal(err)
	}
	calls := 0
	h.bypassSystem = &bypassSystem{run: func(_ context.Context, input, name string, args ...string) ([]byte, error) {
		calls++
		if calls > 1 {
			t.Fatal("foreign resources mutated")
		}
		return []byte(`{"nftables":[{"table":{"family":"ip","name":"clash_manager_bypass","comment":"another-owner"}}]}`), nil
	}}
	if h.cleanupBypass(context.Background(), 0) == nil || !fileExists(h.bypassJournalPath()) {
		t.Fatal("foreign owner deleted")
	}
}
func TestBypassKernelRules(t *testing.T) {
	if os.Getenv("CLASH_TEST_BYPASS_NETNS") != "1" {
		t.Skip("requires isolated Linux network namespace")
	}
	h := testHelper(t)
	ctx := context.Background()
	ops := h.bypassOS()
	for _, mode := range []string{"redir", "tproxy"} {
		s := testBypassPlan(mode)
		rules := bypassRules(s)
		if _, err := ops.run(ctx, rules, "nft", "--check", "-f", "-"); err != nil {
			t.Fatal(err)
		}
		if _, err := ops.run(ctx, rules, "nft", "-f", "-"); err != nil {
			t.Fatal(err)
		}
		raw, err := ops.run(ctx, "", "nft", "-j", "list", "tables")
		if err != nil {
			t.Fatal(err)
		}
		exists, owner, err := bypassTableOwner(raw)
		if err != nil || !exists || owner != "clash-manager:"+s.Token {
			t.Fatalf("ownership list: %s, %v", raw, err)
		}
		if _, err := ops.run(ctx, "", "nft", "delete", "table", "ip", bypassTable); err != nil {
			t.Fatal(err)
		}
	}
	// Verify iproute2's exact representation, used by ownership-checked cleanup.
	if _, err := ops.run(ctx, "", "ip", "-4", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", bypassRoute); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.run(ctx, "", "ip", "-4", "rule", "add", "priority", bypassRoute, "fwmark", bypassMark, "lookup", bypassRoute); err != nil {
		t.Fatal(err)
	}
	s := testBypassPlan("tproxy")
	s.RouteAdded = true
	s.RuleAdded = true
	forward, err := ops.readForward()
	if err != nil {
		t.Fatal(err)
	}
	s.PreviousForward = forward
	if err = h.saveBypassState(s); err != nil {
		t.Fatal(err)
	}
	if err = h.cleanupBypass(ctx, 0); err != nil {
		t.Fatal(err)
	}
}
func TestBypassMonitorIncludesPortsWithoutSecrets(t *testing.T) {
	m, e := monitorConfig([]byte("redir-port: 7895\ntproxy-port: 7896\nsecret: private\n"))
	if e != nil || m["redirPort"] != 7895 || m["tproxyPort"] != 7896 {
		t.Fatal(m, e)
	}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "private") {
		t.Fatal("secret leaked")
	}
}
