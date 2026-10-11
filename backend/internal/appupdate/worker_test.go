package appupdate

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIInstallAndStartKeepSeparateLogs(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "appcenter-cli")
	script := "#!/bin/sh\ncase \"$1\" in\ninstall-fpk) echo '[Info]Install output';;\nstart) echo '[Info]Application [clash-for-fnos] is already started.';;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, args := range [][]string{{"install-fpk", "update.fpk"}, {"start", AppName}} {
		if err := runCLIIn(context.Background(), dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	install, err := os.ReadFile(filepath.Join(dir, "install.log"))
	if err != nil {
		t.Fatal(err)
	}
	start, err := os.ReadFile(filepath.Join(dir, "start.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(install) != "[Info]Install output\n" || !strings.Contains(string(start), "already started") {
		t.Fatalf("installation evidence was lost: install=%q start=%q", install, start)
	}
}

func TestCLIRejectsExplicitErrorWithZeroExit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "appcenter-cli"), []byte("#!/bin/sh\necho '[Error]Installation rejected'\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := runCLIIn(context.Background(), dir, "install-fpk", "update.fpk")
	if err == nil || !strings.Contains(err.Error(), "Installation rejected") {
		t.Fatalf("business error accepted: %v", err)
	}
	output, readErr := os.ReadFile(filepath.Join(dir, "install.log"))
	if readErr != nil || !strings.Contains(string(output), "[Error]") {
		t.Fatalf("missing original error: %q %v", output, readErr)
	}
}

func TestWaitHealthyConfirmsVersionAndPreservesFailureReason(t *testing.T) {
	for _, test := range []struct {
		name, body, detail string
		status             int
		healthy            bool
	}{
		{"new version", `{"ok":true,"version":"v1.3.20"}`, "运行版本 1.3.20", 200, true},
		{"old version still running", `{"ok":true,"version":"1.3.19"}`, "当前运行版本 1.3.19，目标版本 1.3.20", 200, false},
		{"http error", `unavailable`, "HTTP 503", 503, false},
		{"invalid payload", `not json`, "无效数据", 200, false},
		{"not ready", `{"ok":false,"version":"1.3.20"}`, "尚未就绪", 200, false},
		{"missing version", `{"ok":true}`, "没有有效版本", 200, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Keep Unix socket paths short on macOS as well as Linux.
			dir, err := os.MkdirTemp("/tmp", "up-health-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			listener, err := net.Listen("unix", filepath.Join(dir, "clash-for-fnos.sock"))
			if err != nil {
				t.Fatal(err)
			}
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/health" {
					t.Errorf("wrong health route: %s", r.URL.Path)
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})}
			defer server.Close()
			go server.Serve(listener)
			healthy, detail := waitHealthy(context.Background(), dir, "1.3.20", 100*time.Millisecond)
			if healthy != test.healthy || !strings.Contains(detail, test.detail) {
				t.Fatalf("healthy=%v detail=%q", healthy, detail)
			}
		})
	}
}
