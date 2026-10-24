package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

const maxFirewallRuleBytes = 2 << 20

// Keep bounded text while draining the process output; never accept command args
// from an HTTP request. Rules are displayed as text, not executable configuration.
type firewallRuleOutput struct {
	data      bytes.Buffer
	truncated bool
}

func (b *firewallRuleOutput) Len() int       { return b.data.Len() }
func (b *firewallRuleOutput) String() string { return b.data.String() }
func (b *firewallRuleOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxFirewallRuleBytes - b.Len()
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.data.Write(p)
	return n, nil
}
func (h *helper) firewallRules(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nft", "--numeric", "--handle", "list", "ruleset")
	var output firewallRuleOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fail(504, "读取防火墙规则超时，请重试")
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fail(503, "nftables 工具不可用，无法读取规则")
		}
		return nil, fail(502, "无法读取防火墙规则，请检查系统支持与读取权限")
	}
	return map[string]any{"text": strings.ToValidUTF8(output.String(), "�"), "truncated": output.truncated, "readAt": time.Now().UTC().Format(time.RFC3339)}, nil
}
