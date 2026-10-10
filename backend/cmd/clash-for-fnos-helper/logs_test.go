package main

import (
	"bytes"
	"encoding/json"
	"github.com/chenpingonline/Clash-Manager/backend/internal/logstore"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoreOutputKeepsDrainingOnStorageFailure(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "blocked"), []byte("file"), 0600)
	w := &coreLogWriter{store: logstore.New(filepath.Join(dir, "blocked", "log"), logstore.Policy{Days: 7, MaxMiB: 1}, 0600)}
	b := bytes.Repeat([]byte("output\n"), 300000)
	if n, e := w.Write(b); n != len(b) || e != nil || w.lastError == "" {
		t.Fatalf("write=%d %v status=%s", n, e, w.lastError)
	}
}
func TestRunningCoreRotatesAndContinuesAfterClear(t *testing.T) {
	file := filepath.Join(t.TempDir(), "log")
	w := &coreLogWriter{store: logstore.New(file, logstore.Policy{Days: 7, MaxMiB: 1}, 0600)}
	ready := file + ".ready"
	cmd := exec.Command("sh", "-c", `i=0; while [ $i -lt 2000 ]; do printf '%s\n' "$2"; i=$((i+1)); done; echo ready > "$1"; read continuation; echo final >&2`, "fixture", ready, strings.Repeat("x", 1024))
	cmd.Stdout = w
	cmd.Stderr = w
	input, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		input.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Core output phase timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
	stats, e := w.store.Stats()
	if e != nil || stats.Size > logstore.MiB || stats.Files < 2 {
		t.Fatalf("stats=%+v %v", stats, e)
	}
	if e = w.store.Clear(); e != nil {
		t.Fatal(e)
	}
	if _, e = input.Write([]byte("continue\n")); e != nil {
		t.Fatal(e)
	}
	if e = cmd.Wait(); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(file)
	if !strings.Contains(string(b), "final\n") {
		t.Fatalf("after clear=%q", b)
	}
}

func TestCoreLogAPIReconfiguresAndClearsOnlyOwnedOutput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "core", "mihomo.log")
	h := newHelper(helperConfig{etcDir: dir, appDir: dir, managedLog: file})
	history := filepath.Join(dir, "history.log")
	if err := os.WriteFile(history, []byte("history"), 0600); err != nil {
		t.Fatal(err)
	}
	h.coreLogs.Write(bytes.Repeat([]byte("core\n"), 500000))
	settings := logstore.Defaults()
	settings.Core = logstore.Policy{Days: 0, MaxMiB: 1}
	if err := logstore.SaveSettings(logstore.SettingsPath(dir), settings); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/logs/status", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	stats, _ := h.coreLogs.store.Stats()
	if stats.Size > logstore.MiB || stats.MaxBytes != logstore.MiB {
		t.Fatalf("policy not applied %+v", stats)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/logs/clear", strings.NewReader(`{}`)))
	if w.Code != 200 {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
	b, _ := os.ReadFile(history)
	if string(b) != "history" {
		t.Fatal("history modified")
	}
	stats, _ = h.coreLogs.store.Stats()
	if stats.Size != 0 {
		t.Fatal("output retained")
	}
	h.coreLogs.Write([]byte("after-clear\n"))
	b, _ = os.ReadFile(file)
	if string(b) != "after-clear\n" {
		t.Fatal("writer stopped after API clear")
	}
	h.coreLogs.Write([]byte("startup failed\npanic: incomplete"))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/logs/core?limit=1&search=PANIC&file="+history, nil))
	var page logstore.RawPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Lines) != 1 || page.Lines[0].Message != "panic: incomplete" {
		t.Fatalf("raw read %d %s", w.Code, w.Body.String())
	}
	for path, code := range map[string]int{"/logs/core?limit=2001": 400, "/logs/core?cursor=bad": 409} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != code {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
