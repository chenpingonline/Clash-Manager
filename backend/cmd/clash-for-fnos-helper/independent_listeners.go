package main

import (
	"encoding/json"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
)

func decodeIndependentListeners(value any) ([]configyaml.IndependentListener, error) {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return nil, fail(400, "独立代理入口必须为列表")
	}
	var entries []configyaml.IndependentListener
	if err = json.Unmarshal(raw, &entries); err != nil {
		return nil, fail(400, "独立代理入口格式无效")
	}
	if err = configyaml.ValidateIndependentListeners(entries); err != nil {
		return nil, fail(400, err.Error())
	}
	return entries, nil
}
func independentPreferences(settings map[string]any) ([]configyaml.IndependentListener, error) {
	if value, ok := settings[configyaml.IndependentListenersKey]; ok {
		return decodeIndependentListeners(value)
	}
	return []configyaml.IndependentListener{}, nil
}
func (h *helper) independentListenerStatus(raw string) (map[string]any, error) {
	h.mu.Lock()
	settings, _, err := h.readUserSettings()
	h.mu.Unlock()
	if err != nil {
		return nil, err
	}
	entries, err := independentPreferences(settings)
	if err != nil {
		return nil, err
	}
	targets := configyaml.IndependentListenerTargets([]byte(raw))
	known := map[string]bool{}
	for _, name := range targets {
		known[name] = true
	}
	missing := []string{}
	for _, e := range entries {
		if e.Enabled && !known[e.Proxy] {
			missing = append(missing, e.ID)
		}
	}
	return map[string]any{"entries": entries, "targets": targets, "missingTargets": missing}, nil
}
