package logstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawPagesPreservePartialLinesAndSearchArchives(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "core.log"), Policy{0, 4}, 0600)
	var content strings.Builder
	for i := 0; i < 2400; i++ {
		fmt.Fprintf(&content, "Match-%04d %s\r\n", i, strings.Repeat("x", 450))
	}
	content.WriteString("panic: final partial line")
	mustWrite(t, s, []byte(content.String()))
	p, err := s.ReadRawPage(context.Background(), Query{Limit: 1})
	if err != nil || len(p.Lines) != 1 || p.Lines[0].Message != "panic: final partial line" || !p.HasMore {
		t.Fatalf("partial page=%+v err=%v", p, err)
	}
	q := Query{Limit: 2000, Search: "MATCH-"}
	p, err = s.ReadRawPage(context.Background(), q)
	if err != nil || len(p.Lines) != 2000 || !p.HasMore || !strings.HasPrefix(p.Lines[0].Message, "Match-0400 ") {
		t.Fatalf("first len=%d more=%v err=%v", len(p.Lines), p.HasMore, err)
	}
	q.Cursor = p.NextCursor
	mustWrite(t, s, []byte("\nnew append\n"))
	p, err = s.ReadRawPage(context.Background(), q)
	if err != nil || len(p.Lines) != 400 || p.HasMore || !strings.HasPrefix(p.Lines[399].Message, "Match-0399 ") || strings.HasSuffix(p.Lines[399].Message, "\r") {
		t.Fatalf("older len=%d more=%v err=%v", len(p.Lines), p.HasMore, err)
	}
	s.Clear()
	if _, err = s.ReadRawPage(context.Background(), q); !errors.Is(err, ErrCursorExpired) {
		t.Fatalf("cleared cursor=%v", err)
	}
}

func TestRawPayloadBoundAndTruncation(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "core.log"), Policy{0, 10}, 0600)
	for i := 0; i < 80; i++ {
		mustWrite(t, s, []byte(strings.Repeat("x", 40*1024)+"\n"))
	}
	p, err := s.ReadRawPage(context.Background(), Query{Limit: 2000})
	if err != nil || !p.HasMore || len(p.Lines) == 0 {
		t.Fatalf("bounded page len=%d more=%v err=%v", len(p.Lines), p.HasMore, err)
	}
	total, truncated := 0, false
	for _, line := range p.Lines {
		total += len(line.Message)
		truncated = truncated || line.Truncated
		if len(line.Message) > 32*1024 {
			t.Fatal("line exceeded cap")
		}
	}
	if total > int(MiB) || !truncated || p.NextCursor == "" {
		t.Fatalf("payload=%d truncated=%v", total, truncated)
	}
	if _, err = s.ReadRawPage(context.Background(), Query{Cursor: p.NextCursor}); err != nil {
		t.Fatal(err)
	}
}

func TestRawReadIgnoresUnrelatedAndSymlinkFiles(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "core.log"), Policy{0, 1}, 0600)
	os.WriteFile(filepath.Join(dir, "secret"), []byte("private"), 0600)
	os.Symlink(filepath.Join(dir, "secret"), s.File+".part-123")
	os.Symlink(filepath.Join(dir, "secret"), s.File)
	os.WriteFile(s.File+".part-invalid", []byte("unrelated"), 0600)
	p, err := s.ReadRawPage(context.Background(), Query{})
	if err != nil || len(p.Lines) != 0 {
		t.Fatalf("unowned file leaked=%+v %v", p, err)
	}
	for _, c := range []string{"invalid", strings.Repeat("x", 1025)} {
		if _, err = s.ReadRawPage(context.Background(), Query{Cursor: c}); !errors.Is(err, ErrCursorExpired) {
			t.Fatalf("invalid cursor accepted: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	os.Remove(s.File)
	mustWrite(t, s, []byte("owned\n"))
	if _, err = s.ReadRawPage(ctx, Query{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
}
