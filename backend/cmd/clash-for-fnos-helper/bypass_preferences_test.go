package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

type persistentFixture struct {
	h                               *helper
	p                               *bypassPreferences
	pid, installs                   int
	table, route, rule, failForward bool
	owner, forward                  string
	iface                           bypassInterface
}

func newPersistentFixture(t *testing.T) *persistentFixture {
	t.Helper()
	f := &persistentFixture{h: testHelper(t), pid: 123, forward: "0", iface: bypassInterface{"eth0", "192.168.1.2", "192.168.1.0/24"}}
	if err := f.h.writeMode("managed"); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(f.h.config.managedConfig, []byte("allow-lan: true\nbind-address: 192.168.1.2\ntproxy-port: 7896\ntun: {enable: false}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.p = &bypassPreferences{Schema: 1, Mode: "tproxy", Interface: f.iface, Enabled: true, KeepEnabled: true}
	if err := f.h.saveBypassPreferences(f.p); err != nil {
		t.Fatal(err)
	}
	f.h.bypassSystem = &bypassSystem{
		interfaces: func() []bypassInterface { return []bypassInterface{f.iface} },
		process:    func() *processInfo { return &processInfo{PID: f.pid, Managed: true} },
		liveConfig: func(context.Context, []byte) (map[string]any, error) {
			return map[string]any{"tun": map[string]any{"enable": false}, "tproxy-port": float64(7896)}, nil
		},
		probe:       func(context.Context, string) error { return nil },
		readForward: func() (string, error) { return f.forward, nil },
		writeForward: func(v string) error {
			if f.failForward && v == "1" {
				return errors.New("injected forward failure")
			}
			f.forward = v
			return nil
		},
		run: func(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
			switch command := name + " " + strings.Join(args, " "); command {
			case "nft -j list tables":
				if f.table {
					return []byte(fmt.Sprintf(`{"nftables":[{"table":{"family":"ip","name":"clash_manager_bypass","comment":%q}}]}`, f.owner)), nil
				}
				return []byte(`{"nftables":[]}`), nil
			case "nft --check -f -":
				return nil, nil
			case "nft -f -":
				s, err := f.h.readBypassState()
				if err != nil {
					return nil, err
				}
				f.table = true
				f.owner = "clash-manager:" + s.Token
				f.installs++
				return nil, nil
			case "nft delete table ip clash_manager_bypass":
				f.table = false
				return nil, nil
			case "ip -4 rule show":
				if f.rule {
					return []byte("18462: from all fwmark 0x434d/0xffff lookup 18462\n"), nil
				}
				return nil, nil
			case "ip -4 route show table 18462":
				if f.route {
					return []byte("local default dev lo scope host\n"), nil
				}
				return nil, nil
			case "ip -4 route add local 0.0.0.0/0 dev lo table 18462":
				f.route = true
				return nil, nil
			case "ip -4 rule add priority 18462 fwmark 0x434d/0xffff lookup 18462":
				f.rule = true
				return nil, nil
			case "ip -4 rule del priority 18462 fwmark 0x434d/0xffff lookup 18462":
				f.rule = false
				return nil, nil
			case "ip -4 route del local 0.0.0.0/0 dev lo table 18462":
				f.route = false
				return nil, nil
			default:
				t.Fatalf("unexpected command: %s", command)
				return nil, nil
			}
		},
	}
	return f
}
func TestBypassPersistentRestoreAfterCoreRestart(t *testing.T) {
	f := newPersistentFixture(t)
	ctx := context.Background()
	f.h.reconcileBypass(ctx, f.pid)
	if !f.table || !f.route || !f.rule || f.forward != "1" || f.installs != 1 {
		t.Fatal("not installed", f.installs)
	}
	f.h.reconcileBypass(ctx, f.pid)
	if f.installs != 1 {
		t.Fatal("duplicate install")
	}
	if err := f.h.cleanupBypass(ctx, f.pid); err != nil {
		t.Fatal(err)
	}
	p, _ := f.h.readBypassPreferences()
	if !p.Enabled || !p.KeepEnabled {
		t.Fatal("cleanup erased intent")
	}
	f.pid++
	f.h.reconcileBypass(ctx, f.pid)
	s, _ := f.h.readBypassState()
	if f.installs != 2 || s == nil || s.CorePID != f.pid {
		t.Fatal("restart did not restore", s)
	}
	if err := f.h.stopBypass(ctx); err != nil {
		t.Fatal(err)
	}
	f.pid++
	f.h.reconcileBypass(ctx, f.pid)
	if f.table || f.route || f.rule || f.installs != 2 {
		t.Fatal("manual stop restored")
	}
	p, _ = f.h.readBypassPreferences()
	if p.Enabled || p.Mode != "tproxy" || p.Interface != f.iface {
		t.Fatal(p)
	}
}
func TestBypassKeepSwitchPreservesCurrentRules(t *testing.T) {
	f := newPersistentFixture(t)
	ctx := context.Background()
	f.h.reconcileBypass(ctx, f.pid)
	if _, err := f.h.updateBypassKeep(map[string]any{"keepEnabled": false}); err != nil {
		t.Fatal(err)
	}
	if !f.table {
		t.Fatal("keep switch stopped rules")
	}
	if err := f.h.cleanupBypass(ctx, f.pid); err != nil {
		t.Fatal(err)
	}
	f.pid++
	f.h.reconcileBypass(ctx, f.pid)
	if f.table || f.installs != 1 {
		t.Fatal("keep off restored")
	}
}
func TestBypassRestoreRejectsConflictsAndRollsBack(t *testing.T) {
	for _, reason := range []string{"tun", "subnet", "foreign", "forward"} {
		t.Run(reason, func(t *testing.T) {
			f := newPersistentFixture(t)
			switch reason {
			case "tun":
				raw, _ := os.ReadFile(f.h.config.managedConfig)
				_ = atomicWrite(f.h.config.managedConfig, []byte(strings.ReplaceAll(string(raw), "enable: false", "enable: true")), 0600)
			case "subnet":
				f.iface.CIDR = "192.168.0.0/16"
			case "foreign":
				f.table = true
				f.owner = "foreign"
			case "forward":
				f.failForward = true
			}
			f.h.reconcileBypass(context.Background(), f.pid)
			p, err := f.h.readBypassPreferences()
			if err != nil || p.LastError == "" || !p.KeepEnabled {
				t.Fatal("failure not recorded", p, err)
			}
			if f.rule || f.route || (f.table && reason != "foreign") {
				t.Fatal("residual interception")
			}
			if reason == "foreign" && (!f.table || f.owner != "foreign") {
				t.Fatal("foreign rules changed")
			}
			if f.h.bypassManagedStatus(context.Background())["error"] == nil {
				t.Fatal("failure hidden")
			}
		})
	}
}
func TestBypassRestoreWaitsForValidLifecycle(t *testing.T) {
	for _, reason := range []string{"disabled", "keep-off", "old-pid", "transaction", "external"} {
		t.Run(reason, func(t *testing.T) {
			f := newPersistentFixture(t)
			pid := f.pid
			switch reason {
			case "disabled":
				f.p.Enabled = false
				_ = f.h.saveBypassPreferences(f.p)
			case "keep-off":
				f.p.KeepEnabled = false
				_ = f.h.saveBypassPreferences(f.p)
			case "old-pid":
				pid--
			case "transaction":
				f.h.transactions["pending"] = &transaction{Activation: "hot-reload"}
			case "external":
				_ = f.h.writeMode("external")
			}
			f.h.reconcileBypass(context.Background(), pid)
			if f.installs != 0 {
				t.Fatal("invalid lifecycle restored")
			}
		})
	}
}
func TestBypassRuntimeValidationRejectsMismatches(t *testing.T) {
	p := &bypassPreferences{Mode: "tproxy", Interface: bypassInterface{"eth0", "192.168.1.2", "192.168.1.0/24"}}
	raw := []byte("allow-lan: true\nbind-address: 192.168.1.2\ntproxy-port: 7896\ntun: {enable: false}\n")
	for _, live := range []map[string]any{nil, {"tun": map[string]any{"enable": true}, "tproxy-port": float64(7896)}, {"tun": map[string]any{"enable": false}, "tproxy-port": float64(7895)}} {
		if validateBypassRestore(raw, p, live) == nil {
			t.Fatal("mismatch accepted")
		}
	}
}

func TestBypassRestoreDoesNotRewriteEnabledForwarding(t *testing.T) {
	f := newPersistentFixture(t)
	f.forward = "1"
	f.failForward = true
	f.h.reconcileBypass(context.Background(), f.pid)
	if f.installs != 1 || !f.table {
		t.Fatal("already enabled forwarding was rewritten")
	}
	if err := f.h.stopBypass(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.forward != "1" {
		t.Fatal("pre-existing forwarding was changed")
	}
}

func TestBypassUnactivatedCandidateDoesNotBlockRestart(t *testing.T) {
	f := newPersistentFixture(t)
	f.h.transactions["unused-candidate"] = &transaction{}
	f.h.reconcileBypass(context.Background(), f.pid)
	if f.installs != 1 {
		t.Fatal("unused candidate blocked restart restoration")
	}
}
