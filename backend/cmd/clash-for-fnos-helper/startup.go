package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Separate user intent from process liveness: shutdown and package replacement
// must never turn an enabled Core into a deliberately stopped Core.
func (h *helper) saveRunIntent(run bool) error {
	body, err := json.Marshal(map[string]bool{"running": run})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(h.config.etcDir, "core-run-state.json"), body, 0600)
}

func (h *helper) loadRunIntent() (bool, error) {
	body, err := os.ReadFile(filepath.Join(h.config.etcDir, "core-run-state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	} // Legacy installs auto-start.
	if err != nil {
		return false, err
	}
	var state struct {
		Running *bool `json:"running"`
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return false, err
	}
	if state.Running == nil {
		return false, fmt.Errorf("内核运行状态缺少 running 字段")
	}
	return *state.Running, nil
}

func (h *helper) stoppedBootstrap() map[string]any {
	return h.setBootstrap(map[string]any{"state": "stopped", "mode": "managed", "message": "托管 Core 已停止，可点击启动内核或重新检测恢复", "progress": 0})
}

// Each attempt re-reads intent under the operation lock. Manual stop during
// backoff wins; cancellation prevents a shutdown from launching another Core.
func (h *helper) restoreStartup(ctx context.Context, delays []time.Duration) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := h.bootstrapAfterPackageUpgrade(ctx)
		if err == nil {
			return nil
		}
		if h.readMode() != "managed" || attempt >= len(delays) {
			return err
		}
		log.Printf("Mihomo startup attempt %d failed; retrying in %s: %v", attempt+1, delays[attempt], err)
		timer := time.NewTimer(delays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
