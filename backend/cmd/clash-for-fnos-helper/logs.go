package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/logstore"
)

func (h *helper) readCoreLogs(w http.ResponseWriter, r *http.Request) {
	if err := h.refreshLogPolicy(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if r.URL.Query().Get("limit") != "" && (err != nil || limit < 1 || limit > 2000) {
		writeJSON(w, 400, map[string]string{"error": "日志行数须为 1–2000"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := h.coreLogs.store.ReadRawPage(ctx, logstore.Query{Limit: limit, Search: r.URL.Query().Get("search"), Cursor: r.URL.Query().Get("cursor")})
	if err != nil {
		code := 500
		if errors.Is(err, logstore.ErrCursorExpired) {
			code = 409
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	h.coreLogs.mu.Lock()
	page.Error = h.coreLogs.lastError
	h.coreLogs.mu.Unlock()
	writeJSON(w, 200, page)
}

type coreLogWriter struct {
	store       *logstore.Store
	mu          sync.Mutex
	lastError   string
	lastWarning time.Time
}

func (w *coreLogWriter) Write(body []byte) (int, error) {
	_, err := w.store.Write(body)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.lastError = err.Error()
		if time.Since(w.lastWarning) > time.Minute {
			log.Printf("Core log storage failed: %v", err)
			w.lastWarning = time.Now()
		}
	} else {
		w.lastError = ""
	}
	// Always drain Core output. A full/unwritable disk must not block its pipe
	// or interrupt the networking process; surface the storage error in status.
	return len(body), nil
}
func (h *helper) refreshLogPolicy() error {
	settings, err := logstore.LoadSettings(logstore.SettingsPath(h.config.etcDir))
	if err != nil {
		return err
	}
	return h.coreLogs.store.Configure(settings.Core)
}
func (h *helper) coreLogStatus() (map[string]any, error) {
	policyErr := h.refreshLogPolicy()
	stats, err := h.coreLogs.store.Stats()
	h.coreLogs.mu.Lock()
	lastError := h.coreLogs.lastError
	h.coreLogs.mu.Unlock()
	if policyErr != nil {
		lastError = policyErr.Error()
	}
	if err != nil {
		lastError = err.Error()
	}
	return map[string]any{"available": true, "size": stats.Size, "maxBytes": stats.MaxBytes, "files": stats.Files, "oldest": stats.Oldest, "error": lastError}, nil
}
func (h *helper) maintainCoreLogs(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.refreshLogPolicy(); err != nil {
				h.coreLogs.mu.Lock()
				h.coreLogs.lastError = err.Error()
				h.coreLogs.mu.Unlock()
			}
		}
	}
}
