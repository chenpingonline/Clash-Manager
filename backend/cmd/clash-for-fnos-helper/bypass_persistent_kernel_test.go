package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Real Core and nftables, only in a disposable network namespace with no host
// networking. This validates lifecycle restoration, not real client forwarding.
func TestBypassPersistentKernelLifecycle(t *testing.T) {
	core := os.Getenv("CLASH_TEST_REAL_BYPASS_CORE")
	if core == "" || os.Getenv("CLASH_TEST_BYPASS_NETNS") != "1" {
		t.Skip("requires isolated namespace and real Mihomo")
	}
	for _, args := range [][]string{{"link", "add", "lan0", "type", "veth", "peer", "name", "lan1"}, {"addr", "add", "192.168.1.2/24", "dev", "lan0"}, {"link", "set", "lan0", "up"}, {"link", "set", "lan1", "up"}} {
		if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
			t.Fatalf("ip %v: %v %s", args, err, out)
		}
	}
	h := testHelper(t)
	if err := os.MkdirAll(h.config.managedCoreDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(core, h.config.managedCore, 0755); err != nil {
		t.Fatal(err)
	}
	if err := h.writeMode("managed"); err != nil {
		t.Fatal(err)
	}
	raw := []byte("mode: direct\nexternal-controller: 127.0.0.1:19090\nsecret: isolated-test\nallow-lan: true\nbind-address: 192.168.1.2\nmixed-port: 0\ntproxy-port: 7896\ntun:\n  enable: false\ndns:\n  enable: false\nipv6: false\nrules:\n  - MATCH,DIRECT\n")
	if err := atomicWrite(h.config.managedConfig, raw, 0600); err != nil {
		t.Fatal(err)
	}
	p := &bypassPreferences{Schema: 1, Mode: "tproxy", Interface: bypassInterface{"lan0", "192.168.1.2", "192.168.1.0/24"}, Enabled: true, KeepEnabled: true}
	if err := h.saveBypassPreferences(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopManaged)
	h.mu.Lock()
	proc, err := h.startManaged()
	h.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	await := func(pid int) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for {
			h.bypassMu.Lock()
			s, e := h.readBypassState()
			prefs, _ := h.readBypassPreferences()
			h.bypassMu.Unlock()
			if e == nil && s != nil && s.CorePID == pid {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				tables, e := h.bypassOS().run(ctx, "", "nft", "-j", "list", "tables")
				status := h.bypassManagedStatus(ctx, tables)
				cancel()
				if e != nil || status["rulesPresent"] != true {
					t.Fatal(e, status)
				}
				return
			}
			if prefs != nil && prefs.LastError != "" {
				t.Fatal(prefs.LastError)
			}
			if time.Now().After(deadline) {
				t.Fatalf("restore timeout for %d: %v %v", pid, s, e)
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	await(proc.PID)
	result, err := h.restartManaged(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pid := result["pid"].(int)
	if pid == proc.PID {
		t.Fatal("Core PID did not change")
	}
	await(pid)
	// A committed configuration transaction also triggers verification/restoration.
	if err = h.cleanupBypass(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.transactions["kernel-reapply"] = &transaction{}
	h.mu.Unlock()
	if _, err = h.commitConfig("kernel-reapply"); err != nil {
		t.Fatal(err)
	}
	await(pid)
	if err = h.stopBypass(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = h.restartManaged(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.reconcileBypass(context.Background(), 0)
	s, err := h.readBypassState()
	if err != nil || s != nil {
		t.Fatal("explicit stop restored rules", s, err)
	}
	tables, err := h.bypassOS().run(context.Background(), "", "nft", "-j", "list", "tables")
	if err != nil {
		t.Fatal(err)
	}
	exists, _, err := bypassTableOwner(tables)
	if err != nil || exists {
		t.Fatal(fmt.Sprint("residual table: ", string(tables), err))
	}
}
