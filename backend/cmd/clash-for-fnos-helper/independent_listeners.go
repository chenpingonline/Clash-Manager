package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

const independentProfilesKey = "manager-independent-profiles"

type independentConfig struct {
	Enabled bool                             `json:"enabled"`
	Entries []configyaml.IndependentListener `json:"entries"`
}

func (c independentConfig) effective() []configyaml.IndependentListener {
	if c.Enabled {
		return c.Entries
	}
	result := append([]configyaml.IndependentListener{}, c.Entries...)
	for i := range result {
		result[i].Enabled = false
	}
	return result
}

// The profile catalog is authoritative even while config-meta is temporarily
// changed by a profile activation transaction.
func (h *helper) activeIndependentScope() string {
	var state struct {
		Current *string `json:"current"`
	}
	raw, err := os.ReadFile(filepath.Join(h.config.etcDir, "profiles.json"))
	if err == nil && json.Unmarshal(raw, &state) == nil && state.Current != nil && *state.Current != "" {
		return "profile:" + *state.Current
	}
	return "runtime"
}

func validIndependentScope(scope string) bool {
	return scope == "runtime" || (strings.HasPrefix(scope, "profile:") && len(scope) > 8 && len(scope) <= 160 && !strings.ContainsAny(scope, "/\\\r\n\x00"))
}

// Caller holds h.mu. Persist the legacy binding once, before switching profiles.
// Unattributable entries remain available for explicit assignment, never copied.
func (h *helper) independentStore(settings map[string]any) (map[string]independentConfig, error) {
	store := map[string]independentConfig{}
	if value, ok := settings[independentProfilesKey]; ok {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &store); err != nil || store == nil {
			return nil, fmt.Errorf("独立代理入口存储格式无效")
		}
		return store, nil
	}
	if _, legacy := settings[configyaml.IndependentListenersKey]; !legacy {
		return store, nil
	}
	entries, err := independentPreferences(settings)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		scope := h.activeIndependentScope()
		if scope == "runtime" {
			scope = "unassigned"
		}
		store[scope] = independentConfig{Enabled: true, Entries: entries}
	}
	settings[independentProfilesKey] = store
	delete(settings, configyaml.IndependentListenersKey)
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err == nil {
		err = atomicWrite(h.userSettingsPath(), raw, 0600)
	}
	return store, err
}

func scopeConfig(store map[string]independentConfig, scope string) independentConfig {
	if config, exists := store[scope]; exists {
		if config.Entries == nil {
			config.Entries = []configyaml.IndependentListener{}
		}
		return config
	}
	return independentConfig{Enabled: true, Entries: []configyaml.IndependentListener{}}
}

// Apply only ordinary overrides. Management records must never reach Mihomo.
func independentBase(raw string, settings map[string]any) (string, error) {
	overrides := map[string]any{}
	for key, value := range settings {
		if key != configyaml.IndependentListenersKey && key != independentProfilesKey {
			overrides[key] = value
		}
	}
	content, err := configyaml.MergeOverrides([]byte(raw), overrides)
	if err != nil {
		return "", err
	}
	content, err = ensureLANBindWithPreferences(content, settings)
	return string(content), err
}

func independentStatus(raw string, config independentConfig, pending independentConfig) map[string]any {
	targets := configyaml.IndependentListenerTargets([]byte(raw))
	known := map[string]bool{}
	for _, name := range targets {
		known[name] = true
	}
	missing := []string{}
	for _, e := range config.Entries {
		if e.Enabled && !known[e.Proxy] {
			missing = append(missing, e.ID)
		}
	}
	reserved := []int{}
	listeners := networkListeners(raw)
	extra, _ := customNetworkListeners(raw)
	listeners = append(listeners, extra...)
	seen := map[int]bool{}
	for _, listener := range listeners {
		_, port, _ := net.SplitHostPort(listener.address)
		n, _ := strconv.Atoi(port)
		if n > 0 && !seen[n] {
			reserved = append(reserved, n)
			seen[n] = true
		}
	}
	return map[string]any{"enabled": config.Enabled, "entries": config.Entries, "targets": targets, "targetGroups": configyaml.IndependentListenerTargetGroups([]byte(raw)), "missingTargets": missing, "unassigned": pending.Entries, "reservedPorts": reserved}
}

func (h *helper) independentListenerStatus(raw string) (map[string]any, error) {
	return h.readIndependentListeners(map[string]any{"scope": h.activeIndependentScope(), "content": raw})
}

func (h *helper) readIndependentListeners(input map[string]any) (map[string]any, error) {
	scope := stringField(input, "scope")
	if !validIndependentScope(scope) {
		return nil, fail(400, "独立代理入口配置无效")
	}
	raw := stringField(input, "content")
	if scope == "runtime" && raw == "" {
		active, err := h.networkConfig()
		if err != nil {
			return nil, err
		}
		raw = stringField(active, "content")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	settings, _, err := h.readUserSettings()
	if err != nil {
		return nil, err
	}
	store, err := h.independentStore(settings)
	if err != nil {
		return nil, err
	}
	raw, err = independentBase(raw, settings)
	if err != nil {
		return nil, err
	}
	previous := []configyaml.IndependentListener{}
	for _, config := range store {
		previous = append(previous, config.Entries...)
	}
	clean, _, err := configyaml.MergeIndependentListeners([]byte(raw), nil, previous)
	if err != nil {
		return nil, err
	}
	return independentStatus(string(clean), scopeConfig(store, scope), scopeConfig(store, "unassigned")), nil
}

// Inactive profile saves never probe live ports or touch a config transaction.
func (h *helper) saveInactiveIndependentListeners(input map[string]any) (map[string]any, error) {
	scope := stringField(input, "scope")
	if !validIndependentScope(scope) {
		return nil, fail(400, "独立代理入口配置无效")
	}
	entries, err := decodeIndependentListeners(input["entries"])
	if err != nil {
		return nil, err
	}
	enabled, ok := input["enabled"].(bool)
	if !ok {
		return nil, fail(400, "独立代理入口总开关必须为布尔值")
	}
	config := independentConfig{Enabled: enabled, Entries: entries}
	h.mu.Lock()
	defer h.mu.Unlock()
	settings, _, err := h.readUserSettings()
	if err != nil {
		return nil, err
	}
	store, err := h.independentStore(settings)
	if err != nil {
		return nil, err
	}
	raw, err := independentBase(stringField(input, "content"), settings)
	if err != nil {
		return nil, err
	}
	merged, _, err := configyaml.MergeIndependentListeners([]byte(raw), config.effective(), scopeConfig(store, scope).Entries)
	if err != nil {
		return nil, err
	}
	if err = validateConfiguredNetworkPorts(string(merged)); err != nil {
		return nil, err
	}
	store[scope] = config
	if boolField(input, "assignUnassigned") {
		delete(store, "unassigned")
	}
	settings[independentProfilesKey] = store
	body, err := json.MarshalIndent(settings, "", "  ")
	if err == nil {
		err = atomicWrite(h.userSettingsPath(), body, 0600)
	}
	if err != nil {
		return nil, err
	}
	return independentStatus(raw, config, scopeConfig(store, "unassigned")), nil
}

func (h *helper) deleteIndependentListeners(input map[string]any) (map[string]any, error) {
	scope := stringField(input, "scope")
	if !validIndependentScope(scope) {
		return nil, fail(400, "独立代理入口配置无效")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	settings, _, err := h.readUserSettings()
	if err != nil {
		return nil, err
	}
	store, err := h.independentStore(settings)
	if err != nil {
		return nil, err
	}
	delete(store, scope)
	settings[independentProfilesKey] = store
	body, err := json.MarshalIndent(settings, "", "  ")
	if err == nil {
		err = atomicWrite(h.userSettingsPath(), body, 0600)
	}
	return map[string]any{"ok": err == nil}, err
}

func allIndependentEntries(store map[string]independentConfig) []configyaml.IndependentListener {
	entries := []configyaml.IndependentListener{}
	for _, config := range store {
		entries = append(entries, config.Entries...)
	}
	return entries
}
