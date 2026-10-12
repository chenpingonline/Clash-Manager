// Package ruleimport recognizes routing resources without executing imported scripts.
package ruleimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const MaxInput = 8 << 20
const MaxItems = 200000

type Options struct {
	Name      string `json:"name"`
	Policy    string `json:"policy"`
	Behavior  string `json:"behavior"`
	URL       string `json:"url"`
	Placement string `json:"placement"`
	Replace   bool   `json:"replace"`
}
type Issue struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	Content string `json:"content,omitempty"`
}
type Provider struct {
	Name     string         `json:"name"`
	Behavior string         `json:"behavior"`
	Format   string         `json:"format"`
	Count    int            `json:"count"`
	Sample   []string       `json:"sample"`
	Config   map[string]any `json:"config"`
	Asset    []byte         `json:"-"`
}
type Result struct {
	Format    string           `json:"format"`
	Providers []Provider       `json:"providers"`
	Rules     []string         `json:"rules"`
	Groups    []map[string]any `json:"groups"`
	Warnings  []Issue          `json:"warnings"`
	Errors    []Issue          `json:"errors"`
	Replace   bool             `json:"replace"`
}
type Fetch func(string) ([]byte, error)

func (r *Result) warn(line int, message, content string) {
	if len(r.Warnings) < 100 {
		r.Warnings = append(r.Warnings, Issue{line, message, truncate(content, 300)})
	}
}
func (r *Result) fail(line int, message, content string) {
	if len(r.Errors) < 100 {
		r.Errors = append(r.Errors, Issue{line, message, truncate(content, 300)})
	}
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func Name(s string) string {
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`[\x00-\x1f,\\/]`).ReplaceAllString(s, "_")
	if s == "" {
		s = "Imported"
	}
	return truncate(s, 80)
}
func URL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", errors.New("规则链接必须是 HTTP/HTTPS 地址，且不能包含用户名密码")
	}
	if u.Hostname() == "github.com" {
		p := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(p) >= 5 && p[2] == "blob" {
			u.Host = "raw.githubusercontent.com"
			u.Path = "/" + strings.Join(append(p[:2], p[3:]...), "/")
		}
	}
	u.Fragment = ""
	return u.String(), nil
}
func Parse(raw []byte, o Options, fetch Fetch) (*Result, error) {
	if len(raw) == 0 || len(raw) > MaxInput {
		return nil, errors.New("导入内容为空或超过 8 MiB")
	}
	if o.Policy == "" {
		o.Policy = "DIRECT"
	}
	if strings.ContainsAny(o.Policy, ",\r\n") {
		return nil, errors.New("目标策略无效")
	}
	if o.Behavior != "" && o.Behavior != "auto" && o.Behavior != "domain" && o.Behavior != "ipcidr" && o.Behavior != "classical" {
		return nil, errors.New("规则类型无效")
	}
	r := &Result{Providers: []Provider{}, Rules: []string{}, Groups: []map[string]any{}, Warnings: []Issue{}, Errors: []Issue{}, Replace: o.Replace}
	// MRS is a Zstandard frame; the uncompressed signature identifies its version.
	if bytes.HasPrefix(raw, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		p, err := parseMRS(raw, o)
		if err != nil {
			return nil, err
		}
		r.Format = "MRS"
		r.Providers = append(r.Providers, p)
		r.Rules = append(r.Rules, "RULE-SET,"+p.Name+","+o.Policy)
		return r, nil
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, errors.New("无法识别二进制规则文件；当前支持 YAML、文本、MRS 和 ACL4SSR INI")
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if strings.HasPrefix(strings.ToLower(text), "<!doctype") || strings.HasPrefix(strings.ToLower(text), "<html") {
		return nil, errors.New("链接返回了网页，请使用规则文件的 Raw 地址")
	}
	if strings.Contains(text, "[custom]") {
		return parseINI(text, o, fetch, r)
	}
	var object map[string]any
	err := yaml.Unmarshal([]byte(text), &object)
	if err == nil && object != nil {
		if payload, ok := object["payload"]; ok {
			items, err := stringsList(payload)
			if err != nil {
				return nil, err
			}
			r.Format = "YAML"
			addPayload(r, items, o, "yaml")
			return r, nil
		}
		if _, ok := object["rules"]; ok {
			return parseConfig(object, o, fetch, r)
		}
		if _, ok := object["rule-providers"]; ok {
			return parseConfig(object, o, fetch, r)
		}
		if _, ok := object["proxy-groups"]; ok {
			return parseConfig(object, o, fetch, r)
		}
		return nil, errors.New("YAML 未包含 payload、rules、rule-providers 或 proxy-groups")
	}
	// An explicit YAML marker must not fall back to a domain list on syntax errors.
	if strings.HasPrefix(text, "payload:") || strings.HasPrefix(text, "rules:") || strings.HasPrefix(text, "rule-providers:") {
		return nil, fmt.Errorf("规则 YAML 无效: %v", err)
	}
	var items []string
	if strings.HasPrefix(text, "- ") || strings.HasPrefix(text, "[") {
		if err := yaml.Unmarshal([]byte(text), &items); err != nil {
			return nil, errors.New("规则列表 YAML 无效")
		}
		r.Format = "YAML"
		addPayload(r, items, o, "yaml")
		return r, nil
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		items = append(items, line)
	}
	r.Format = "Text"
	// Full routing lists carry their own policies. Reject mixtures rather than
	// dropping policy columns or turning a catch-all into a provider entry.
	full := false
	for _, item := range items {
		if parts := Parts(item); FullRule(item) == nil && (parts[0] == "MATCH" || len(parts) > 2 && parts[2] != "no-resolve") {
			full = true
			break
		}
	}
	if full {
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			if err := FullRule(line); err != nil {
				r.fail(i+1, err.Error(), line)
			} else {
				r.Rules = append(r.Rules, line)
			}
		}
		if o.URL != "" {
			r.warn(0, "完整规则按当前内容保存；自动更新仅适用于远程规则集", "")
		}
		return r, nil
	}
	addPayload(r, items, o, "text")
	// Diagnostics refer to the original text, including comment and blank lines.
	lineNumbers := []int{}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, ";") {
			lineNumbers = append(lineNumbers, i+1)
		}
	}
	for i := range r.Warnings {
		if n := r.Warnings[i].Line; n > 0 && n <= len(lineNumbers) {
			r.Warnings[i].Line = lineNumbers[n-1]
		}
	}
	return r, nil
}
func stringsList(value any) ([]string, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("规则内容必须是字符串列表")
	}
	if len(list) > MaxItems {
		return nil, errors.New("规则条目超过 200000 条")
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("规则内容必须是字符串列表")
		}
		out = append(out, s)
	}
	return out, nil
}

var ruleTypes = map[string]bool{}

func init() {
	for _, s := range strings.Fields("DOMAIN DOMAIN-SUFFIX DOMAIN-KEYWORD DOMAIN-REGEX GEOSITE GEOIP SRC-GEOIP IP-ASN SRC-IP-ASN IP-CIDR IP-CIDR6 SRC-IP-CIDR SRC-IP-CIDR6 IP-SUFFIX SRC-IP-SUFFIX SRC-PORT DST-PORT IN-PORT DSCP PROCESS-NAME PROCESS-PATH PROCESS-NAME-REGEX PROCESS-PATH-REGEX NETWORK UID IN-TYPE IN-USER IN-NAME AND OR NOT RULE-SET MATCH") {
		ruleTypes[s] = true
	}
}
func Parts(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
		}
		if c == ',' && depth == 0 {
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}
func domain(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return false
	}
	if strings.ContainsAny(s, " ,:/\t\r\n[]{}=^|@#;") || s == "" {
		return false
	}
	// Retain Mihomo's exact, suffix-only, '+.' and '*' wildcard semantics.
	d := strings.TrimPrefix(strings.TrimPrefix(s, "+."), ".")
	if strings.Contains(d, "..") || strings.HasSuffix(d, ".") {
		return false
	}
	return strings.Contains(d, ".") || regexp.MustCompile(`^[A-Za-z0-9_*-]+$`).MatchString(d)
}
func infer(s string) string {
	if _, err := netip.ParsePrefix(s); err == nil {
		return "ipcidr"
	}
	p := Parts(s)
	if len(p) > 1 {
		return "classical"
	}
	if domain(s) {
		return "domain"
	}
	return ""
}
func classical(s string) (string, error) {
	p := Parts(s)
	if len(p) < 2 || !ruleTypes[p[0]] || p[0] == "RULE-SET" || p[0] == "MATCH" {
		return "", errors.New("不支持的规则或无法识别的语法")
	}
	if len(p) > 3 || len(p) == 3 && p[2] != "no-resolve" {
		return "", errors.New("规则集内不能携带目标策略，请使用完整 rules 配置或移除策略列")
	}
	if p[1] == "" {
		return "", errors.New("规则匹配内容为空")
	}
	if strings.Contains(p[0], "CIDR") {
		if _, err := netip.ParsePrefix(p[1]); err != nil {
			return "", errors.New("IP 网段无效")
		}
	}
	if strings.HasSuffix(p[0], "REGEX") {
		if _, err := regexp.Compile(p[1]); err != nil {
			return "", errors.New("正则表达式无效")
		}
	}
	return strings.Join(p, ","), nil
}
func addPayload(r *Result, items []string, o Options, format string) {
	behavior := o.Behavior
	if behavior == "auto" {
		behavior = ""
	}
	if len(items) > MaxItems {
		r.fail(0, "规则条目超过 200000 条", "")
		return
	}
	if behavior == "" {
		for _, s := range items {
			b := infer(strings.TrimSpace(s))
			if b == "" {
				continue
			}
			if behavior == "" {
				behavior = b
			} else if behavior != b {
				r.fail(0, "规则列表混合了域名、网段或完整规则，请分别导入或明确选择规则类型", "")
				return
			}
		}
	}
	if behavior == "" {
		r.fail(0, "没有可识别的规则", "")
		return
	}
	clean := []string{}
	seen := map[string]bool{}
	for i, s := range items {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		valid := false
		var err error
		switch behavior {
		case "domain":
			valid = domain(s)
		case "ipcidr":
			_, err = netip.ParsePrefix(s)
			valid = err == nil
		case "classical":
			s, err = classical(s)
			valid = err == nil
		}
		if !valid {
			message := "无效的规则条目"
			if err != nil {
				message = err.Error()
			}
			r.warn(i+1, message, items[i])
			continue
		}
		if !seen[s] {
			seen[s] = true
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		r.fail(0, "没有可导入的有效规则", "")
		return
	}
	config := map[string]any{"type": "inline", "behavior": behavior, "payload": clean}
	if o.URL != "" {
		config = map[string]any{"type": "http", "behavior": behavior, "format": format, "url": o.URL, "interval": 86400, "path": "./rule-import/" + assetID([]byte(o.URL)) + "." + format}
		for i := range r.Warnings {
			if behavior == "classical" && legacyUnsupported(r.Warnings[i].Content) {
				r.Warnings[i].Message = "Mihomo 不支持此规则，远程更新时也会跳过该条目"
			} else {
				r.fail(0, "远程规则包含无法加载的条目，请修正来源或以文本方式导入有效内容", "")
				break
			}
		}
	}
	p := Provider{Name: Name(o.Name), Behavior: behavior, Format: format, Count: len(clean), Sample: sample(clean), Config: config}
	r.Providers = append(r.Providers, p)
	r.Rules = append(r.Rules, "RULE-SET,"+p.Name+","+o.Policy)
}
func assetID(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:16]) }
func sample(items []string) []string {
	if len(items) > 20 {
		return items[:20]
	}
	return items
}
func parseConfig(config map[string]any, o Options, fetch Fetch, r *Result) (*Result, error) {
	r.Format = "Clash YAML"
	for key := range config {
		if key != "rules" && key != "rule-providers" && key != "proxy-groups" {
			r.warn(0, "只导入规则、规则集和代理组，此配置项已忽略", key)
		}
	}
	if rules, ok := config["rules"]; ok {
		list, err := stringsList(rules)
		if err != nil {
			return nil, err
		}
		for i, s := range list {
			if err := FullRule(s); err != nil {
				r.warn(i+1, err.Error(), s)
			} else {
				r.Rules = append(r.Rules, s)
			}
		}
	}
	if groups, ok := config["proxy-groups"]; ok {
		list, ok := groups.([]any)
		if !ok {
			return nil, errors.New("proxy-groups 必须是列表")
		}
		for _, v := range list {
			g, ok := v.(map[string]any)
			if !ok {
				return nil, errors.New("代理组必须是对象")
			}
			r.Groups = append(r.Groups, g)
		}
	}
	if providers, ok := config["rule-providers"]; ok {
		m, ok := providers.(map[string]any)
		if !ok {
			return nil, errors.New("rule-providers 必须是对象")
		}
		if len(m) > 64 {
			return nil, errors.New("一次最多导入 64 个规则集")
		}
		for _, name := range sortedKeys(m) {
			v, ok := m[name].(map[string]any)
			if !ok {
				return nil, errors.New("规则集配置必须是对象")
			}
			b, _ := v["behavior"].(string)
			f, _ := v["format"].(string)
			typ, _ := v["type"].(string)
			if f == "" {
				f = "yaml"
			}
			switch typ {
			case "inline":
				items, err := stringsList(v["payload"])
				if err != nil {
					return nil, err
				}
				tmp := &Result{}
				addPayload(tmp, items, Options{Name: name, Policy: o.Policy, Behavior: b}, "yaml")
				r.Warnings = append(r.Warnings, tmp.Warnings...)
				r.Errors = append(r.Errors, tmp.Errors...)
				r.Providers = append(r.Providers, tmp.Providers...)
			case "http":
				rawURL, _ := v["url"].(string)
				u, err := URL(rawURL)
				if err != nil {
					r.fail(0, err.Error(), name)
					continue
				}
				p, issues, err := remoteProvider(name, u, b, f, fetch)
				if err != nil {
					r.fail(0, err.Error(), name)
					continue
				}
				r.Warnings = append(r.Warnings, issues...)
				if interval, ok := v["interval"]; ok {
					p.Config["interval"] = interval
				}
				r.Providers = append(r.Providers, p)
			default:
				r.fail(0, "配置片段仅支持 inline 和 http 规则集；本地 file 来源请单独上传", name)
			}
		}
	}
	if len(r.Rules) == 0 && len(r.Providers) > 0 {
		for _, p := range r.Providers {
			r.Rules = append(r.Rules, "RULE-SET,"+p.Name+","+o.Policy)
		}
	}
	if len(r.Rules)+len(r.Providers)+len(r.Groups) == 0 {
		return nil, errors.New("未找到可导入的规则、规则集或代理组")
	}
	return r, nil
}
func FullRule(s string) error {
	if strings.ContainsAny(s, "\r\n\x00") {
		return errors.New("规则包含无效字符")
	}
	p := Parts(s)
	if len(p) < 2 || !ruleTypes[p[0]] {
		return errors.New("不支持的规则类型")
	}
	policy := 1
	if p[0] != "MATCH" {
		policy = 2
	}
	if len(p) <= policy || p[policy] == "" {
		return errors.New("规则缺少目标策略")
	}
	if len(p) > policy+2 || len(p) == policy+2 && p[policy+1] != "no-resolve" {
		return errors.New("规则附加参数无效")
	}
	if p[0] != "MATCH" && p[0] != "RULE-SET" {
		if _, err := classical(strings.Join(p[:2], ",")); err != nil {
			return err
		}
	}
	if p[0] == "RULE-SET" && p[1] == "" {
		return errors.New("规则匹配内容为空")
	}
	return nil
}
func remoteProvider(name, u, b, f string, fetch Fetch) (Provider, []Issue, error) {
	if fetch == nil {
		return Provider{}, nil, errors.New("无法下载远程规则集")
	}
	raw, err := fetch(u)
	if err != nil {
		return Provider{}, nil, fmt.Errorf("下载规则集失败: %s", name)
	}
	parsed, err := Parse(raw, Options{Name: name, Policy: "DIRECT", Behavior: b}, nil)
	if err != nil {
		return Provider{}, nil, err
	}
	if len(parsed.Errors) > 0 || len(parsed.Providers) != 1 || parsed.Format == "Clash YAML" {
		return Provider{}, nil, errors.New("远程来源不是有效的单个规则集")
	}
	for i := range parsed.Warnings {
		if parsed.Providers[0].Behavior != "classical" || !legacyUnsupported(parsed.Warnings[i].Content) {
			return Provider{}, nil, errors.New("远程规则包含无法转换的条目")
		}
		parsed.Warnings[i].Message = "Mihomo 不支持此规则，远程更新时也会跳过该条目"
	}
	p := parsed.Providers[0]
	if b != "" && b != "auto" && p.Behavior != b {
		return Provider{}, nil, errors.New("声明的规则类型与来源内容不匹配")
	}
	if f != "" && f != p.Format {
		return Provider{}, nil, errors.New("声明的规则格式与来源内容不匹配")
	}
	p.Asset = nil
	p.Config = map[string]any{"type": "http", "url": u, "behavior": p.Behavior, "format": p.Format, "interval": 86400, "path": "./rule-import/" + assetID([]byte(u)) + "." + p.Format}
	return p, parsed.Warnings, nil
}

// Common ACL4SSR/Surge lists include these legacy matchers. Mihomo's
// classical provider parser skips them without rejecting the resource. Retain
// the update URL and expose every omission, instead of promising a conversion.
func legacyUnsupported(s string) bool {
	p := Parts(s)
	return len(p) > 1 && (p[0] == "URL-REGEX" || p[0] == "USER-AGENT")
}
