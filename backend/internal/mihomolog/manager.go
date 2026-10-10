package mihomolog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/chenpingonline/Clash-Manager/backend/internal/logstore"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

type Record = logstore.Record

type subscriber struct {
	level   string
	channel chan Record
}

type Manager struct {
	File      string
	mu        sync.RWMutex
	store     *logstore.Store
	saveLevel string
	lastError string
	subsMu    sync.Mutex
	subs      map[*subscriber]struct{}
}

func New(file string) *Manager {
	return &Manager{File: file, store: logstore.New(file, logstore.Defaults().History, 0o600), saveLevel: "info", subs: map[*subscriber]struct{}{}}
}

func normalizeLevel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "warn" {
		return "warning"
	}
	switch value {
	case "debug", "info", "warning", "error":
		return value
	}
	return "info"
}
func rank(value string) int {
	return map[string]int{"debug": 10, "info": 20, "warning": 30, "error": 40}[normalizeLevel(value)]
}

func Normalize(line []byte) (Record, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Record{}, false
	}
	var raw map[string]any
	if json.Unmarshal(line, &raw) != nil {
		raw = map[string]any{"payload": string(line)}
	}
	text := func(key string) string {
		if value, ok := raw[key]; ok && value != nil {
			return fmt.Sprint(value)
		}
		return ""
	}
	when := text("time")
	if when == "" {
		when = time.Now().UTC().Format(time.RFC3339Nano)
	}
	level := text("level")
	if level == "" {
		level = text("type")
	}
	message := text("message")
	if message == "" {
		message = text("payload")
	}
	if message == "" {
		message = string(line)
	}
	return Record{Time: when, Level: normalizeLevel(level), Message: message}, true
}

func (m *Manager) Configure(policy logstore.Policy, level string) error {
	m.mu.Lock()
	m.saveLevel = level
	m.mu.Unlock()
	if err := m.store.Configure(policy); err != nil {
		return err
	}
	return m.store.PruneRecords()
}
func (m *Manager) Append(record Record) error {
	m.mu.RLock()
	level := m.saveLevel
	m.mu.RUnlock()
	if rank(record.Level) < rank(level) {
		return nil
	}
	// Bound each structured record, including adversarially long Controller messages.
	if len(record.Message) > 32*1024 {
		record.Message = strings.ToValidUTF8(record.Message[:32*1024], "") + "…"
	}
	if len(record.Time) > 128 {
		record.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	record.Level = normalizeLevel(record.Level)
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = m.store.Write(append(body, '\n'))
	m.mu.Lock()
	m.lastError = ""
	if err != nil {
		m.lastError = err.Error()
	}
	m.mu.Unlock()
	return err
}
func (m *Manager) History(level string, limit int) (map[string]any, error) {
	page, err := m.Query(context.Background(), logstore.Query{Level: level, Limit: limit})
	return map[string]any{"items": page.Items, "size": page.Size, "maxBytes": page.MaxBytes, "nextCursor": page.NextCursor, "hasMore": page.HasMore, "oldest": page.Oldest}, err
}
func (m *Manager) Query(ctx context.Context, query logstore.Query) (logstore.Page, error) {
	page, err := m.store.ReadPage(ctx, query)
	m.mu.RLock()
	page.Error = m.lastError
	m.mu.RUnlock()
	return page, err
}
func (m *Manager) Stats() (logstore.Stats, error) {
	stats, err := m.store.RecordStats()
	m.mu.RLock()
	stats.Error = m.lastError
	m.mu.RUnlock()
	return stats, err
}
func (m *Manager) Cleanup() error {
	if err := m.store.Cleanup(); err != nil {
		return err
	}
	return m.store.PruneRecords()
}
func (m *Manager) Clear() error { return m.store.Clear() }

func (m *Manager) broadcast(record Record) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	for item := range m.subs {
		if rank(record.Level) >= rank(item.level) {
			select {
			case item.channel <- record:
			default:
			}
		}
	}
}

func (m *Manager) ServeSSE(w http.ResponseWriter, r *http.Request, level string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	item := &subscriber{level: normalizeLevel(level), channel: make(chan Record, 64)}
	m.subsMu.Lock()
	m.subs[item] = struct{}{}
	m.subsMu.Unlock()
	defer func() { m.subsMu.Lock(); delete(m.subs, item); m.subsMu.Unlock() }()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case record := <-item.channel:
			body, _ := json.Marshal(record)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
			flusher.Flush()
		case <-ticker.C:
			_, _ = io.WriteString(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (m *Manager) Run(ctx context.Context, settingsFile string) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.Cleanup()
			}
		}
	}()
	_ = m.Cleanup()
	for ctx.Err() == nil {
		client := &mihomo.Client{SettingsFile: settingsFile}
		streamContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		response, err := client.Do(streamContext, http.MethodGet, "/logs?level=debug&format=structured", nil, 0)
		if err == nil {
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			for scanner.Scan() {
				record, ok := Normalize(scanner.Bytes())
				if ok {
					_ = m.Append(record)
					m.broadcast(record)
				}
			}
			response.Body.Close()
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

func ParseLimit(value string) int {
	number, err := strconv.Atoi(value)
	if err != nil {
		return 800
	}
	return number
}
