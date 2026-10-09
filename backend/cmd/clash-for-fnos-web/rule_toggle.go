package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

type toggleRule struct {
	Index   *int   `json:"index"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
	Extra   *struct {
		Disabled bool `json:"disabled"`
	} `json:"extra"`
}

func findToggleRule(body []byte, index int) (*toggleRule, error) {
	var payload struct {
		Rules []toggleRule `json:"rules"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for _, rule := range payload.Rules {
		if rule.Index != nil && *rule.Index == index {
			return &rule, nil
		}
	}
	return nil, fmt.Errorf("规则不存在或内核不支持规则开关，请刷新规则并检查内核版本")
}

func (g *gateway) setRuleDisabled(w http.ResponseWriter, r *http.Request, live *mihomo.Client) {
	var edit struct {
		Scope    string `json:"scope"`
		Index    *int   `json:"index"`
		Disabled *bool  `json:"disabled"`
		Type     string `json:"type"`
		Payload  string `json:"payload"`
		Proxy    string `json:"proxy"`
	}
	if !decodeJSONBody(w, r, &edit) {
		return
	}
	if edit.Index == nil || *edit.Index < 0 || edit.Disabled == nil || edit.Type == "" || edit.Proxy == "" {
		writeJSON(w, 400, map[string]string{"error": "规则标识或开关状态无效"})
		return
	}
	g.configMu.Lock()
	defer g.configMu.Unlock()
	scope, err := g.currentRuleScope()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if edit.Scope != scope {
		writeJSON(w, 409, map[string]string{"error": "当前订阅已切换，请刷新规则后重试"})
		return
	}
	client, err := live.Snapshot()
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	// Ignore cached rules when mutating: the index may now refer to another rule.
	body, err := g.rulesSnapshot.refresh(r.Context(), client)
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	rule, err := findToggleRule(body, *edit.Index)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	if rule.Extra == nil {
		writeJSON(w, 409, map[string]string{"error": "当前内核不支持此规则的开关，请升级内核后重试"})
		return
	}
	if rule.Type != edit.Type || rule.Payload != edit.Payload || rule.Proxy != edit.Proxy {
		writeJSON(w, 409, map[string]string{"error": "规则已变化，请刷新列表后重试"})
		return
	}
	keys, _, err := ruleStateKeys(body)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	state, err := g.loadRuleState()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "读取规则开关记录失败: " + err.Error()})
		return
	}
	oldDisabled := rule.Extra.Disabled
	patch, _ := json.Marshal(map[int]bool{*edit.Index: *edit.Disabled})
	if err = mihomoRequest(r.Context(), client, http.MethodPatch, "/rules/disable", bytes.NewReader(patch), 12*time.Second); err != nil {
		writeMihomoError(w, err)
		return
	}
	body, err = g.rulesSnapshot.refresh(r.Context(), client)
	if err == nil {
		var actual *toggleRule
		actual, err = findToggleRule(body, *edit.Index)
		if err == nil && (actual.Type != edit.Type || actual.Payload != edit.Payload || actual.Proxy != edit.Proxy || actual.Extra == nil || actual.Extra.Disabled != *edit.Disabled) {
			err = fmt.Errorf("内核未返回预期的规则状态，请刷新列表确认")
		}
		if err == nil {
			actualKeys, _, keyErr := ruleStateKeys(body)
			if keyErr != nil || actualKeys[*edit.Index] != keys[*edit.Index] {
				err = fmt.Errorf("规则列表已变化，未保存开关设置，请刷新后重试")
			}
		}
	}
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "规则开关请求已发送，但确认状态失败: " + err.Error()})
		return
	}
	if *edit.Disabled {
		state[keys[*edit.Index]] = true
	} else {
		delete(state, keys[*edit.Index])
	}
	if err := g.saveRuleState(state); err != nil {
		// Do not report a durable success when only the runtime was changed.
		rollback, _ := json.Marshal(map[int]bool{*edit.Index: oldDisabled})
		rollbackErr := mihomoRequest(r.Context(), client, http.MethodPatch, "/rules/disable", bytes.NewReader(rollback), 12*time.Second)
		_, _ = g.rulesSnapshot.refresh(r.Context(), client)
		message := "保存规则开关失败: " + err.Error()
		if rollbackErr != nil {
			message += "；恢复原运行状态失败: " + rollbackErr.Error()
		}
		writeJSON(w, 500, map[string]string{"error": message})
		return
	}
	body, err = g.bindRuleScope(body, scope)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeRawJSON(w, 200, body, "mihomo")
}
