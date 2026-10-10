package mihomolog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryFiltersLevelAndLimit(t *testing.T) {
	manager := New(filepath.Join(t.TempDir(), "mihomo.log"))
	for _, record := range []Record{{Time: "1", Level: "info", Message: "a"}, {Time: "2", Level: "warning", Message: "b"}, {Time: "3", Level: "error", Message: "c"}} {
		if err := manager.Append(record); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := manager.History("warning", 1)
	if err != nil {
		t.Fatal(err)
	}
	items := payload["items"].([]Record)
	if len(items) != 1 || items[0].Message != "c" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if err := manager.Clear(); err != nil {
		t.Fatal(err)
	}
	payload, _ = manager.History("debug", 10)
	if len(payload["items"].([]Record)) != 0 {
		t.Fatal("history was not cleared")
	}
}

func TestNormalizeStructuredAndPlainLogs(t *testing.T) {
	record, ok := Normalize([]byte(`{"time":"now","type":"warn","payload":"message"}`))
	if !ok || record.Level != "warning" || record.Message != "message" {
		t.Fatalf("unexpected record: %#v", record)
	}
	record, ok = Normalize([]byte("plain"))
	if !ok || record.Level != "info" || record.Message != "plain" {
		t.Fatalf("unexpected plain record: %#v", record)
	}
}

func TestStorageLevelDoesNotSuppressLiveDebug(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "log"))
	sub := &subscriber{level: "debug", channel: make(chan Record, 1)}
	m.subs[sub] = struct{}{}
	record := Record{Time: "now", Level: "debug", Message: "diagnostic"}
	if err := m.Append(record); err != nil {
		t.Fatal(err)
	}
	m.broadcast(record)
	select {
	case got := <-sub.channel:
		if got.Message != "diagnostic" {
			t.Fatal(got)
		}
	default:
		t.Fatal("debug live event suppressed")
	}
	page, _ := m.History("debug", 10)
	if len(page["items"].([]Record)) != 0 {
		t.Fatal("debug stored at info default")
	}
}

func TestCollectorStillBroadcastsWhenHistoryCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(filepath.Join(blocked, "history.log"))
	sub := &subscriber{level: "debug", channel: make(chan Record, 1)}
	m.subs[sub] = struct{}{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"time":"now","level":"info","payload":"still-live"}` + "\n"))
	}))
	defer core.Close()
	cfg := filepath.Join(dir, "settings.json")
	body, _ := json.Marshal(map[string]string{"controller": core.URL})
	if err := os.WriteFile(cfg, body, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, cfg); close(done) }()
	defer func() { cancel(); <-done }()
	select {
	case got := <-sub.channel:
		if got.Message != "still-live" {
			t.Fatal(got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("storage failure suppressed live logs")
	}
	m.mu.RLock()
	storageError := m.lastError
	m.mu.RUnlock()
	if storageError == "" {
		t.Fatal("storage error not exposed")
	}
}
