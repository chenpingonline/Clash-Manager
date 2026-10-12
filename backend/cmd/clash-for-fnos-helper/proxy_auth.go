package main

import (
	"context"

	"gopkg.in/yaml.v3"
)

// A direct YAML edit is authoritative for authentication. If the GUI previously
// saved overrides, advance those fields in the same rollbackable transaction so
// a later subscription update cannot resurrect the old credentials.
func (h *helper) prepareStartupConfig(ctx context.Context, content string, validate bool) (map[string]any, error) {
	h.mu.Lock()
	settings, _, err := h.readUserSettings()
	h.mu.Unlock()
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err = yaml.Unmarshal([]byte(content), &document); err != nil {
		return nil, err
	}
	patch := map[string]any{}
	for _, key := range []string{"authentication", "skip-auth-prefixes"} {
		if _, owned := settings[key]; !owned {
			continue
		}
		value := document[key]
		if value == nil {
			value = []string{}
		}
		patch[key] = value
	}
	prepared, err := h.prepareConfigCandidate(ctx, content, validate, false)
	if err == nil && len(patch) > 0 {
		h.attachUserSettings(prepared, patch)
	}
	return prepared, err
}
