package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/chenpingonline/Clash-for-fnos/backend/internal/mihomo"
)

// The stored keys contain content, duplicate count and occurrence, never the
// runtime index. An insertion or deletion cannot shift a choice onto another rule.
func ruleStateKeys(body []byte) (map[int]string, map[int]toggleRule, error) {
	var payload struct {
		Rules []toggleRule `json:"rules"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	identity := func(rule toggleRule) string {
		value, _ := json.Marshal([]string{rule.Type, rule.Payload, rule.Proxy})
		return string(value)
	}
	counts, seen := map[string]int{}, map[string]int{}
	for _, rule := range payload.Rules {
		counts[identity(rule)]++
	}
	keys, rules := map[int]string{}, map[int]toggleRule{}
	for _, rule := range payload.Rules {
		key := identity(rule)
		seen[key]++
		if rule.Index != nil && rule.Extra != nil {
			keys[*rule.Index] = fmt.Sprintf("%s/%d/%d", key, counts[key], seen[key])
			rules[*rule.Index] = rule
		}
	}
	return keys, rules, nil
}

// Scope uses the immutable profile ID, so renaming or updating a subscription
// keeps its choices while an identical rule in another subscription stays independent.
func (g *gateway) currentRuleScope() (string, error) {
	body, err := os.ReadFile(g.config.configMetaFile)
	if os.IsNotExist(err) || g.config.configMetaFile == "" {
		return "managed", nil
	}
	if err != nil {
		return "", err
	}
	var meta struct {
		Source   string `json:"source"`
		SourceID string `json:"sourceId"`
		Active   *bool  `json:"active"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", fmt.Errorf("读取当前订阅标识失败: %w", err)
	}
	// Importing a draft without applying it changes config-meta, but the Core
	// still belongs to the last successfully selected subscription.
	if meta.Active != nil && !*meta.Active {
		state, err := g.readProfiles()
		if err != nil {
			return "", err
		}
		if state.Current != nil && *state.Current != "" {
			return "profile:" + *state.Current, nil
		}
		return "managed", nil
	}
	if meta.Source == "profile" && meta.SourceID != "" {
		return "profile:" + meta.SourceID, nil
	}
	return "managed", nil
}

type ruleStateDocument struct {
	Version  int                        `json:"version"`
	Profiles map[string]map[string]bool `json:"profiles"`
	Disabled map[string]bool            `json:"disabled,omitempty"`
}

// Callers hold configMu. Legacy global records are bound once to the currently
// active subscription; they are never copied to every subscription.
func (g *gateway) readRuleStateDocument(scope string) (ruleStateDocument, error) {
	empty := ruleStateDocument{Version: 2, Profiles: map[string]map[string]bool{}}
	if g.config.ruleStateFile == "" {
		return empty, nil
	}
	body, err := os.ReadFile(g.config.ruleStateFile)
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	var document ruleStateDocument
	if len(body) > 4<<20 || json.Unmarshal(body, &document) != nil {
		return empty, fmt.Errorf("保存的规则开关记录无效")
	}
	if document.Version == 1 && document.Disabled != nil {
		empty.Profiles[scope] = document.Disabled
		if err := g.writeRuleStateDocument(empty); err != nil {
			return empty, err
		}
		return empty, nil
	}
	if document.Version != 2 || document.Profiles == nil {
		return empty, fmt.Errorf("保存的规则开关记录无效")
	}
	return document, nil
}

func (g *gateway) loadRuleState() (map[string]bool, error) {
	scope, err := g.currentRuleScope()
	if err != nil {
		return nil, err
	}
	document, err := g.readRuleStateDocument(scope)
	if err != nil {
		return nil, err
	}
	if state := document.Profiles[scope]; state != nil {
		return state, nil
	}
	return map[string]bool{}, nil
}

func (g *gateway) writeRuleStateDocument(document ruleStateDocument) error {
	if g.config.ruleStateFile == "" {
		return fmt.Errorf("规则开关保存路径未配置")
	}
	body, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if len(body) > 4<<20 {
		return fmt.Errorf("保存的规则开关记录过大")
	}
	return writeAtomicFile(g.config.ruleStateFile, body)
}

func (g *gateway) saveRuleState(state map[string]bool) error {
	scope, err := g.currentRuleScope()
	if err != nil {
		return err
	}
	document, err := g.readRuleStateDocument(scope)
	if err != nil {
		return err
	}
	document.Profiles[scope] = state
	return g.writeRuleStateDocument(document)
}

func (g *gateway) bindRuleScope(body []byte, scope string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["scope"], _ = json.Marshal(scope)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := g.rulesSnapshot.save(body); err != nil {
		return nil, err
	}
	return body, nil
}

// All restores and manual changes share configMu with config application.
func (g *gateway) refreshRules(ctx context.Context, live *mihomo.Client) (result []byte, resultErr error) {
	g.configMu.Lock()
	defer g.configMu.Unlock()
	scope, err := g.currentRuleScope()
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr == nil {
			result, resultErr = g.bindRuleScope(result, scope)
		}
	}()
	client, err := live.Snapshot()
	if err != nil {
		return nil, err
	}
	state, err := g.loadRuleState()
	if err != nil {
		return nil, fmt.Errorf("读取规则开关记录失败: %w", err)
	}
	body, err := g.rulesSnapshot.refresh(ctx, client)
	if err != nil || len(state) == 0 {
		return body, err
	}
	keys, rules, err := ruleStateKeys(body)
	if err != nil {
		return nil, err
	}
	patch := map[int]bool{}
	for index, key := range keys {
		if state[key] && !rules[index].Extra.Disabled {
			patch[index] = true
		}
	}
	if len(patch) == 0 {
		return body, nil
	}
	raw, _ := json.Marshal(patch)
	if err := mihomoRequest(ctx, client, http.MethodPatch, "/rules/disable", bytes.NewReader(raw), 12*time.Second); err != nil {
		return nil, fmt.Errorf("恢复规则开关失败: %w", err)
	}
	body, err = g.rulesSnapshot.refresh(ctx, client)
	if err != nil {
		return nil, err
	}
	actualKeys, actualRules, err := ruleStateKeys(body)
	if err != nil {
		return nil, err
	}
	for index := range patch {
		if actualKeys[index] != keys[index] || actualRules[index].Extra == nil || !actualRules[index].Extra.Disabled {
			return nil, fmt.Errorf("恢复规则开关后内核状态未确认，将自动重试")
		}
	}
	return body, nil
}

// Polling also covers external Core restarts, helper update/recovery paths and
// failed restores. Saved choices survive errors and are retried without a UI.
func (g *gateway) watchRuleState(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.configMu.Lock()
			state, err := g.loadRuleState()
			g.configMu.Unlock()
			if err != nil {
				log.Printf("读取规则开关记录失败: %v", err)
				continue
			}
			if len(state) == 0 {
				continue
			}
			checkCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
			_, err = g.refreshRules(checkCtx, &mihomo.Client{SettingsFile: g.config.settingsFile})
			cancel()
			if err != nil {
				log.Printf("恢复规则开关失败: %v", err)
			}
		}
	}
}
