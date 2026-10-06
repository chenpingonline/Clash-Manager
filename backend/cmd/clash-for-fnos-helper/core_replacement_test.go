package main

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestManagedExecutableAfterReplacement(t *testing.T) {
	path := "/var/apps/clash-for-fnos/var/mihomo"
	for _, tc := range []struct {
		exe  string
		want bool
	}{
		{path, true}, {path + " (deleted)", true},
		{"/usr/local/bin/mihomo (deleted)", false},
		{path + ".verified (deleted)", false}, {path + " (deleted).other", false}, {"", false},
	} {
		if got := managedExecutable(tc.exe, path); got != tc.want {
			t.Fatalf("%q: %v", tc.exe, got)
		}
	}
}

// Run a real ELF executable which owns a controller-like TCP listener.
func TestCoreReplacementFixture(t *testing.T) {
	if os.Getenv("CLASH_CORE_REPLACEMENT_FIXTURE") != "1" {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.WriteFile(os.Getenv("CLASH_CORE_REPLACEMENT_READY"), []byte(listener.Addr().String()), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Second)
}

func TestReplacedManagedCoreIsStoppedAndReleasesPort(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux /proc executable links")
	}
	for _, missingPID := range []bool{false, true} {
		t.Run(strconv.FormatBool(missingPID), func(t *testing.T) {
			h := testHelper(t)
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if err := copyFile(binary, h.config.managedCore, 0755); err != nil {
				t.Fatal(err)
			}
			ready := h.config.managedCore + ".ready"
			cmd := exec.Command(h.config.managedCore, "-test.run=^TestCoreReplacementFixture$", "--", "-f", h.config.managedConfig, "-d", h.config.managedConfigDir)
			cmd.Env = append(os.Environ(), "CLASH_CORE_REPLACEMENT_FIXTURE=1", "CLASH_CORE_REPLACEMENT_READY="+ready)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			defer func() { _ = cmd.Process.Kill(); <-done }()
			var address string
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if raw, err := os.ReadFile(ready); err == nil {
					address = string(raw)
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if address == "" {
				t.Fatal("fixture did not open listener")
			}
			if !missingPID {
				if err := atomicWrite(h.config.managedPID, []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := copyFile(binary, h.config.managedCore, 0755); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Readlink("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/exe")
			if err != nil || !strings.HasSuffix(exe, " (deleted)") {
				t.Fatalf("exe=%q err=%v", exe, err)
			}
			proc := h.primary()
			if proc == nil || !proc.Managed || proc.Exe != h.config.managedCore {
				t.Fatalf("lost managed core: %#v", proc)
			}
			h.stopManaged()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("old core still running")
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("controller port not released: %v", err)
			}
			_ = listener.Close()
		})
	}
}
