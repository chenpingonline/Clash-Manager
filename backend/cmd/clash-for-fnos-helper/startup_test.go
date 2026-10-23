package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStartupCoreProcess(t *testing.T) {
	if os.Getenv("CLASH_TEST_STARTUP_CORE") != "1" {
		return
	}
	path := os.Getenv("CLASH_TEST_ATTEMPTS")
	body, _ := os.ReadFile(path)
	body = append(body, 'x')
	if os.WriteFile(path, body, 0600) != nil {
		os.Exit(2)
	}
	if len(body) < 3 {
		os.Exit(42)
	}
	server := &http.Server{Addr: os.Getenv("CLASH_TEST_CONTROLLER"), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"version":"test"}`)) })}
	_ = server.ListenAndServe()
	os.Exit(0)
}

func failingStartupHelper(t *testing.T) *helper {
	t.Helper()
	h := testHelper(t)
	if err := atomicWrite(h.config.managedCore, []byte("#!/bin/sh\nexit 42\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.config.managedConfig, []byte("external-controller: 127.0.0.1:0\nmixed-port: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestStartupIntentSurvivesHelperRestart(t *testing.T) {
	h := failingStartupHelper(t)
	if run, err := h.loadRunIntent(); !run || err != nil {
		t.Fatalf("legacy default: %t %v", run, err)
	}
	if _, err := h.stopCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(h.config.varDir, "bundled-core-upgrade.pending")
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	restarted := newHelper(h.config)
	if err := restarted.restoreStartup(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if restarted.bootstrapSnapshot()["state"] != "stopped" {
		t.Fatal(restarted.bootstrapSnapshot())
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("stopped Core consumed pending upgrade")
	}
	// Explicit start records intent even if the current launch fails.
	if _, err := restarted.startCore(context.Background()); err == nil {
		t.Fatal("expected early exit")
	}
	restarted.stopManaged() // Host shutdown must preserve enabled intent.
	if run, err := newHelper(h.config).loadRunIntent(); !run || err != nil {
		t.Fatalf("lost intent: %t %v", run, err)
	}
}

func TestStartupRetriesRecover(t *testing.T) {
	h := failingStartupHelper(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	attempts := filepath.Join(h.config.etcDir, "attempts")
	t.Setenv("CLASH_TEST_STARTUP_CORE", "1")
	t.Setenv("CLASH_TEST_ATTEMPTS", attempts)
	t.Setenv("CLASH_TEST_CONTROLLER", address)
	script := fmt.Sprintf("#!/bin/sh\nexec '%s' -test.run=^TestStartupCoreProcess$\n", strings.ReplaceAll(executable, "'", "'\"'\"'"))
	if err := atomicWrite(h.config.managedCore, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.config.managedConfig, []byte("external-controller: "+address+"\nmixed-port: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		body, _ := os.ReadFile(h.config.managedPID)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
		if pid > 1 {
			p, _ := os.FindProcess(pid)
			_ = p.Kill()
		}
	}()
	if err := h.restoreStartup(context.Background(), []time.Duration{time.Millisecond, time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(attempts)
	if string(body) != "xxx" || h.bootstrapSnapshot()["state"] != "ready" {
		t.Fatalf("attempts=%s state=%v", body, h.bootstrapSnapshot())
	}
}

func TestStartupStopAndCancelDuringRetry(t *testing.T) {
	for _, stop := range []bool{true, false} {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			h := failingStartupHelper(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- h.restoreStartup(ctx, []time.Duration{200 * time.Millisecond}) }()
			deadline := time.Now().Add(5 * time.Second)
			for h.bootstrapSnapshot()["state"] != "error" {
				if time.Now().After(deadline) {
					t.Fatal("no initial attempt")
				}
				time.Sleep(time.Millisecond)
			}
			if stop {
				if _, err := h.stopCore(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if stop && (err != nil || h.bootstrapSnapshot()["state"] != "stopped") {
					t.Fatalf("stop lost: %v %v", err, h.bootstrapSnapshot())
				}
				if !stop && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("retry did not finish")
			}
		})
	}
}

func TestStartupRetryExhaustionAndInvalidIntent(t *testing.T) {
	h := failingStartupHelper(t)
	if err := h.restoreStartup(context.Background(), []time.Duration{time.Millisecond}); err == nil {
		t.Fatal("expected final failure")
	}
	if h.bootstrapSnapshot()["state"] != "error" {
		t.Fatal(h.bootstrapSnapshot())
	}
	if err := os.WriteFile(filepath.Join(h.config.etcDir, "core-run-state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.loadRunIntent(); err == nil {
		t.Fatal("invalid intent silently enabled Core")
	}
	if err := h.restoreStartup(context.Background(), nil); err == nil || h.bootstrapSnapshot()["state"] != "error" {
		t.Fatal("invalid intent not reported")
	}
}

func TestStartupControllerTimeoutCleansProcess(t *testing.T) {
	h := failingStartupHelper(t)
	if err := atomicWrite(h.config.managedCore, []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if h.managed != nil {
			_ = h.managed.Kill()
		}
	}()
	if _, err := h.startManaged(); err == nil {
		t.Fatal("unresponsive core considered ready")
	}
	if h.managed == nil {
		t.Fatal("core was never launched")
	}
	if err := h.managed.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("timed-out core left running")
	}
}
