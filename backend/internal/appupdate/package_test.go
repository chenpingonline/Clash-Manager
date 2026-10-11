package appupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, fields, extra string) (string, string) {
	t.Helper()
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	archive := tar.NewWriter(gz)
	payload := []byte("test application payload")
	manifest := fmt.Sprintf("appname = clash-for-fnos\nversion = 1.3.16\nplatform = x86\nchecksum = %x\n", md5.Sum(payload)) + fields
	for _, entry := range []struct {
		name string
		data []byte
	}{{"manifest", []byte(manifest)}, {"app.tgz", payload}, {extra, []byte("other")}} {
		if entry.name == "" {
			continue
		}
		if err := archive.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "test.fpk")
	if err := os.WriteFile(filename, body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename, fmt.Sprintf("sha256:%x", sha256.Sum256(body.Bytes()))
}

func TestPackageVerification(t *testing.T) {
	t.Parallel()
	file, digest := fixture(t, "", "")
	if err := Verify(file, digest, "v1.3.16", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, digest, target, arch string }{{"digest", "sha256:" + strings.Repeat("0", 64), "1.3.16", "amd64"}, {"no digest", "", "1.3.16", "amd64"}, {"version", digest, "1.3.17", "amd64"}, {"architecture", digest, "1.3.16", "arm64"}, {"unsupported", digest, "1.3.16", "386"}} {
		t.Run(test.name, func(t *testing.T) {
			if err := Verify(file, test.digest, test.target, test.arch); err == nil {
				t.Fatal("unsafe package accepted")
			}
		})
	}
	for _, extra := range []string{"../escape", "/absolute", "manifest", "dir/../escape"} {
		file, digest := fixture(t, "", extra)
		if err := Verify(file, digest, "1.3.16", "amd64"); err == nil {
			t.Fatalf("accepted %s", extra)
		}
	}
	file, digest = fixture(t, "appname = another-app\n", "")
	if err := Verify(file, digest, "1.3.16", "amd64"); err == nil {
		t.Fatal("duplicate appname accepted")
	}
}

func TestReleaseVersionsAndURLs(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"1.3.16", "v1.3.16", "0.1.0"} {
		if _, err := Version(v); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{"latest", "1.2", "v1.3.16-rc1", "1.03.16", "1.3.16; rm", "https://example.test"} {
		if _, err := Version(v); err == nil {
			t.Fatal(v)
		}
	}
	if !Newer("1.3.16", "1.3.15") || Newer("1.3.15", "1.3.15") || Newer("1.2.99", "1.3.15") {
		t.Fatal("incorrect version ordering")
	}
	for _, raw := range []string{"https://github.com/a", "https://release-assets.githubusercontent.com/a"} {
		if !trustedURL(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"http://github.com/a", "https://github.com.evil.test/a", "https://user@github.com/a", "https://github.com:444/a", "file:///tmp/a"} {
		if trustedURL(raw) {
			t.Fatal(raw)
		}
	}
}

func TestDownloadRejectsIncompleteAndOversize(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"abc", "abcde", "abcdef"} {
		t.Run(content, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.(http.Flusher).Flush(); io.WriteString(w, content) }))
			defer server.Close()
			var downloaded int64
			err := Download(context.Background(), server.Client(), asset{URL: server.URL, Size: 5}, filepath.Join(t.TempDir(), "update.fpk"), func(n int64) { downloaded = n })
			if (err == nil) != (len(content) == 5) {
				t.Fatalf("content=%s err=%v", content, err)
			}
			if len(content) == 5 && downloaded != 5 {
				t.Fatal(downloaded)
			}
		})
	}
}

func TestStatusSurvivesNewReaderAndWorkerLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteStatus(dir, Status{Stage: "installing", TargetVersion: "1.3.16", Progress: 88}); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(dir)
	if err != nil || !status.Active() || status.TargetVersion != "1.3.16" || status.UpdatedAt == 0 {
		t.Fatalf("%+v %v", status, err)
	}
	unlock, err := lockWorker(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if second, err := lockWorker(dir); err == nil {
		second()
		t.Fatal("duplicate worker accepted")
	}
}

func TestActualFPK(t *testing.T) {
	file := os.Getenv("CLASH_UPDATE_FPK_FIXTURE")
	if file == "" {
		t.Skip("optional existing package verification")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if err := Verify(file, "sha256:"+hex.EncodeToString(digest[:]), os.Getenv("CLASH_UPDATE_FPK_VERSION"), "amd64"); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPipelineRejectsBeforeInvokingAppCenter(t *testing.T) {
	t.Parallel()
	file, _ := fixture(t, "", "")
	installed := false
	err := installVerified(context.Background(), file, "sha256:"+strings.Repeat("0", 64), "1.3.16", "amd64", func(string, int) error { return nil }, func() error { installed = true; return nil }, func() error { return nil }, func(time.Duration) bool { return true })
	if err == nil || installed {
		t.Fatalf("unverified package installed: %v", err)
	}
}

func TestInstallPipelineAndRecovery(t *testing.T) {
	t.Parallel()
	file, digest := fixture(t, "", "")
	for _, test := range []struct {
		name       string
		installErr error
		recovery   bool
		healthy    bool
	}{{"normal", nil, false, true}, {"start required", nil, true, true}, {"install failure", errors.New("installer failed"), false, true}, {"health failure", nil, true, false}} {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			checks := 0
			err := installVerified(context.Background(), file, digest, "1.3.16", "amd64", func(stage string, _ int) error { events = append(events, stage); return nil }, func() error { events = append(events, "cli-install"); return test.installErr }, func() error { events = append(events, "cli-start"); return nil }, func(time.Duration) bool { checks++; return test.healthy && (!test.recovery || checks > 1) })
			if (err != nil) != (test.installErr != nil || !test.healthy) {
				t.Fatalf("%v: %v", events, err)
			}
			joined := strings.Join(events, ",")
			if !strings.HasPrefix(joined, "verifying,installing,cli-install") {
				t.Fatal(joined)
			}
			if test.installErr != nil && strings.Contains(joined, "restarting") {
				t.Fatal(joined)
			}
			if !test.healthy && strings.Contains(joined, "success") {
				t.Fatal(joined)
			}
			if test.installErr == nil && test.healthy && !strings.HasSuffix(joined, "success") {
				t.Fatal(joined)
			}
		})
	}
}
