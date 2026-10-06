package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func seedUpgradeCore(t *testing.T, h *helper, current, bundled string, invalidConfig bool) {
	t.Helper()
	script := func(version string, reject bool) []byte {
		testExit := 0
		if reject {
			testExit = 1
		}
		return []byte(fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n-v) echo 'Mihomo Meta %s';;\n-t) exit %d;;\n*) exit 1;;\nesac\n", version, testExit))
	}
	if err := atomicWrite(h.config.managedCore, script(current, false), 0755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.config.appDir, "core")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	writer := gzip.NewWriter(&data)
	writer.Write(script(bundled, invalidConfig))
	writer.Close()
	sum := sha256.Sum256(data.Bytes())
	meta, _ := json.Marshal(map[string]any{"tag": bundled, "size": data.Len(), "sha256": hex.EncodeToString(sum[:])})
	for name, body := range map[string][]byte{"bundled-core.json": meta, "mihomo-linux-test.gz": data.Bytes()} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(h.config.varDir, "bundled-core-upgrade.pending"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPackageUpgradeBundledPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, current, bundled string
		update                 bool
	}{
		{"older", "v1.19.30", "v1.19.32", true}, {"same", "v1.19.32", "v1.19.32", false},
		{"newer online core", "v1.20.0", "v1.19.32", false}, {"numeric patch", "v1.19.9", "v1.19.32", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHelper(t)
			seedUpgradeCore(t, h, tc.current, tc.bundled, false)
			backup, err := h.upgradeBundledLocked(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := tc.current
			if tc.update {
				want = tc.bundled
			}
			if got := readVersion(h.config.managedCore); got != want {
				t.Fatalf("version=%s want=%s", got, want)
			}
			if (backup != "") != tc.update {
				t.Fatalf("backup=%q", backup)
			}
			if tc.update && readVersion(backup) != tc.current {
				t.Fatal("backup lost old core")
			}
		})
	}
}

func TestPackageUpgradePreservesCoreWhenNotApplicable(t *testing.T) {
	for _, name := range []string{"ordinary restart", "external", "universal"} {
		t.Run(name, func(t *testing.T) {
			h := testHelper(t)
			seedUpgradeCore(t, h, "v1.19.30", "v1.19.32", false)
			switch name {
			case "ordinary restart":
				os.Remove(filepath.Join(h.config.varDir, "bundled-core-upgrade.pending"))
			case "external":
				if err := h.writeMode("external"); err != nil {
					t.Fatal(err)
				}
			case "universal":
				os.Remove(filepath.Join(h.config.appDir, "core", "bundled-core.json"))
			}
			backup, err := h.upgradeBundledLocked(context.Background())
			if err != nil || backup != "" || readVersion(h.config.managedCore) != "v1.19.30" {
				t.Fatalf("backup=%q err=%v", backup, err)
			}
		})
	}
}

func TestPackageUpgradeValidationFailurePreservesCore(t *testing.T) {
	for _, name := range []string{"digest", "config", "unknown installed version"} {
		t.Run(name, func(t *testing.T) {
			h := testHelper(t)
			current := "v1.19.30"
			if name == "unknown installed version" {
				current = "alpha"
			}
			seedUpgradeCore(t, h, current, "v1.19.32", name == "config")
			before, _ := os.ReadFile(h.config.managedCore)
			if name == "digest" {
				os.WriteFile(filepath.Join(h.config.appDir, "core", "mihomo-linux-test.gz"), []byte("broken"), 0600)
			}
			if name == "config" {
				atomicWrite(h.config.managedConfig, []byte("mixed-port: 0\n"), 0600)
			}
			backup, err := h.upgradeBundledLocked(context.Background())
			after, _ := os.ReadFile(h.config.managedCore)
			if err == nil || backup != "" || !bytes.Equal(before, after) {
				t.Fatalf("backup=%q err=%v core changed=%v", backup, err, !bytes.Equal(before, after))
			}
		})
	}
}

func TestPackageUpgradeBootstrapFailureRollsBack(t *testing.T) {
	h := testHelper(t)
	seedUpgradeCore(t, h, "v1.19.30", "v1.19.32", false)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := atomicWrite(h.config.managedConfig, []byte(fmt.Sprintf("external-controller: %s\nmixed-port: 0\n", listener.Addr())), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = h.bootstrapAfterPackageUpgrade(context.Background())
	if err == nil {
		t.Fatal("expected port conflict")
	}
	if got := readVersion(h.config.managedCore); got != "v1.19.30" {
		t.Fatalf("old core not restored: %s", got)
	}
	if !fileExists(filepath.Join(h.config.varDir, "bundled-core-upgrade.pending")) {
		t.Fatal("failed upgrade must remain retryable")
	}
}
