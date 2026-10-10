package logstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustWrite(t *testing.T, s *Store, b []byte) {
	t.Helper()
	if n, e := s.Write(b); e != nil || n != len(b) {
		t.Fatalf("write=%d/%d %v", n, len(b), e)
	}
}
func TestCapacityRotationAndLegacyWriter(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mihomo.log")
	s := New(file, Policy{7, 1}, 0600)
	mustWrite(t, s, bytes.Repeat([]byte("entry\n"), 300000))
	stats, e := s.Stats()
	if e != nil || stats.Size > MiB || stats.Files < 2 {
		t.Fatalf("stats=%+v %v", stats, e)
	}
	// An adopted process retains its append descriptor while cleanup truncates.
	f, e := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = f.Write(bytes.Repeat([]byte("legacy\n"), 200000)); e != nil {
		t.Fatal(e)
	}
	if e = s.Cleanup(); e != nil {
		t.Fatal(e)
	}
	if e = s.Clear(); e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write([]byte("still-running\n")); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "still-running\n" {
		t.Fatalf("descriptor lost: %q", b)
	}
}
func TestExpiryAndUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mihomo.log")
	s := New(file, Policy{7, 20}, 0600)
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	for _, when := range []time.Time{old, now} {
		b, _ := json.Marshal(Record{when.Format(time.RFC3339Nano), "info", "retained"})
		mustWrite(t, s, append(b, '\n'))
	}
	unrelated := filepath.Join(dir, "other.log")
	os.WriteFile(unrelated, []byte("preserve"), 0600)
	os.Symlink(unrelated, file+".part-123")
	if e := s.PruneRecords(); e != nil {
		t.Fatal(e)
	}
	p, e := s.ReadPage(context.Background(), Query{Level: "info"})
	if e != nil || len(p.Items) != 1 || p.Oldest != now.Format(time.RFC3339Nano) {
		t.Fatalf("page=%+v %v", p, e)
	}
	archive := file + ".part-100"
	os.WriteFile(archive, []byte("expired"), 0600)
	os.Chtimes(archive, old, old)
	if e = s.Cleanup(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(archive); !os.IsNotExist(e) {
		t.Fatal("expired archive retained")
	}
	if e = s.Clear(); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(unrelated); string(b) != "preserve" {
		t.Fatal("unrelated file changed")
	}
}
func TestPagedSearchAcrossRotationAndAppend(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "mihomo.log"), Policy{7, 4}, 0600)
	now := time.Now().UTC()
	when := now.Add(-24 * time.Hour).Format(time.RFC3339Nano)
	for i := 0; i < 2505; i++ {
		b, _ := json.Marshal(Record{when, "info", fmt.Sprintf("match-%04d %s", i, strings.Repeat("x", 450))})
		mustWrite(t, s, append(b, '\n'))
	}
	q := Query{Level: "info", Search: "MATCH", Limit: 2000, From: now.Add(-48 * time.Hour), To: now}
	p, e := s.ReadPage(context.Background(), q)
	if e != nil || len(p.Items) != 2000 || !p.HasMore || !strings.HasPrefix(p.Items[0].Message, "match-0505") {
		t.Fatalf("first len=%d more=%v %v", len(p.Items), p.HasMore, e)
	}
	b, _ := json.Marshal(Record{when, "info", "new-append"})
	mustWrite(t, s, append(b, '\n'))
	q.Cursor = p.NextCursor
	p, e = s.ReadPage(context.Background(), q)
	if e != nil || len(p.Items) != 505 || p.HasMore || !strings.HasPrefix(p.Items[504].Message, "match-0504") {
		t.Fatalf("second=%d more=%v %v", len(p.Items), p.HasMore, e)
	}
	s.Clear()
	if _, e = s.ReadPage(context.Background(), q); !errors.Is(e, ErrCursorExpired) {
		t.Fatalf("stale cursor=%v", e)
	}
}
func TestSettingsPersistenceAndValidation(t *testing.T) {
	file := SettingsPath(t.TempDir())
	s, e := LoadSettings(file)
	if e != nil || s != Defaults() {
		t.Fatalf("default=%+v %v", s, e)
	}
	s.History = Policy{0, 1}
	s.SaveLevel = "debug"
	if e = SaveSettings(file, s); e != nil {
		t.Fatal(e)
	}
	got, e := LoadSettings(file)
	if e != nil || got != s {
		t.Fatalf("reload=%+v %v", got, e)
	}
	for _, p := range []Policy{{-1, 1}, {366, 1}, {1, 0}, {1, 1025}} {
		bad := s
		bad.Core = p
		if SaveSettings(file, bad) == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	got, _ = LoadSettings(file)
	if got != s {
		t.Fatal("invalid update replaced settings")
	}
	os.WriteFile(file, []byte("invalid"), 0600)
	if _, e = LoadSettings(file); e == nil {
		t.Fatal("corrupt config hidden")
	}
}
func TestActiveSymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	os.WriteFile(target, []byte("safe"), 0600)
	s := New(filepath.Join(dir, "mihomo.log"), Policy{7, 1}, 0600)
	os.Symlink(target, s.File)
	if _, e := s.Write([]byte("bad")); e == nil {
		t.Fatal("symlink written")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "safe" {
		t.Fatal("target changed")
	}
}
