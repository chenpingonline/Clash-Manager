package main

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var bypassTextHeader = regexp.MustCompile(`^table\s+ip\s+clash_manager_bypass\s*\{\s*(?:#.*)?$`)
var bypassTextComment = regexp.MustCompile(`^comment\s+("(?:[^"\\]|\\.)*")\s*;?\s*(?:#.*)?$`)

// nftables before 1.0.8 can omit table comments from JSON. Only fall back
// when the named table exists and JSON has no comment; explicit foreign
// ownership is never overridden by text output or by a matching table name.
func (h *helper) bypassOwner(ctx context.Context, raw []byte) (bool, string, error) {
	exists, owner, err := bypassTableOwner(raw)
	if err != nil || !exists || owner != "" {
		return exists, owner, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	text, err := h.bypassOS().run(ctx, "", "nft", "-nn", "list", "table", "ip", bypassTable)
	if err != nil {
		return true, "", errors.New("无法核对接管表归属，已保留规则")
	}
	owner, err = bypassOwnerFromText(string(text))
	return true, owner, err
}
func bypassOwnerFromText(text string) (string, error) {
	depth, found := 0, false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if !found {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if !bypassTextHeader.MatchString(trimmed) {
				break
			}
			found = true
		} else if depth == 1 {
			if match := bypassTextComment.FindStringSubmatch(trimmed); match != nil {
				return strconv.Unquote(match[1])
			}
		}
		// Count only structural braces, ignoring quoted comments and set strings.
		quoted, escaped := false, false
		for _, c := range line {
			if escaped {
				escaped = false
				continue
			}
			if quoted && c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				quoted = !quoted
				continue
			}
			if quoted {
				continue
			}
			if c == '#' {
				break
			}
			if c == '{' {
				depth++
			}
			if c == '}' {
				depth--
			}
		}
		if depth <= 0 {
			break
		}
	}
	return "", errors.New("未读到可核验的接管表归属标记，已保留规则")
}
