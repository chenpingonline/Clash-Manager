package ruleimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/dlclark/regexp2"
	"gopkg.in/yaml.v3"
)

type Plan struct {
	*Result
	Extensions        map[string]string `json:"extensions"`
	RuleCount         int               `json:"ruleCount"`
	ProviderRuleCount int               `json:"providerRuleCount"`
	Assets            map[string][]byte `json:"-"`
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func object(raw []byte) (map[string]any, error) {
	var m map[string]any
	err := yaml.Unmarshal(raw, &m)
	if err != nil || m == nil {
		return nil, errors.New("订阅配置必须是 YAML 对象")
	}
	return m, nil
}
func cloneMap(m map[string]any) map[string]any {
	raw, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
func mapping(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return m
}
func values(v any) []any { out, _ := v.([]any); return out }
func identity(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	s, _ := mapping(v)["name"].(string)
	return s
}
func sequence(content string) (configyaml.SequenceExtension, error) {
	s := configyaml.SequenceExtension{Prepend: []any{}, Append: []any{}, Delete: []string{}}
	if content != "" {
		if err := yaml.Unmarshal([]byte(content), &s); err != nil {
			return s, err
		}
	}
	return s, nil
}
func marshal(v any) (string, error) {
	raw, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	if len(raw) > configyaml.MaxExtensionSize {
		return "", errors.New("导入后单个增强文件超过 1 MiB；大型列表请使用远程链接")
	}
	return string(raw), nil
}
func containsString(v []any, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

// Build preserves the original subscription and all unrelated enhancement keys.
// Preview and save use the same complete extension contents, guarded by the gateway.
func Build(r *Result, o Options, raw []byte, profileName string, global, existing map[string]string, assetPath func([]byte) string) (*Plan, error) {
	base, err := object(raw)
	if err != nil {
		return nil, err
	}
	effectiveRaw, err := configyaml.ApplyProfileExtensionChain(raw, profileName, global, existing)
	if err != nil {
		return nil, err
	}
	effective, err := object(effectiveRaw)
	if err != nil {
		return nil, err
	}
	p := &Plan{Result: r, Extensions: map[string]string{}, Assets: map[string][]byte{}, RuleCount: len(r.Rules)}
	override := map[string]any{}
	if existing["override"] != "" {
		if err := yaml.Unmarshal([]byte(existing["override"]), &override); err != nil {
			return nil, err
		}
		if override == nil {
			override = map[string]any{}
		}
	}
	providers := mapping(override["rule-providers"])
	occupied := mapping(effective["rule-providers"])
	for i := range r.Providers {
		provider := &r.Providers[i]
		p.ProviderRuleCount += provider.Count
		if provider.Name == "" || strings.ContainsAny(provider.Name, ",\r\n") {
			r.fail(0, "规则集名称无效", provider.Name)
			continue
		}
		if len(provider.Asset) > 0 {
			path := assetPath(provider.Asset)
			p.Assets[path] = provider.Asset
			provider.Config["path"] = path
		}
		if old, ok := occupied[provider.Name]; ok && !reflect.DeepEqual(cloneMap(mapping(old)), cloneMap(provider.Config)) {
			r.fail(0, "规则集名称与已有配置冲突，请修改导入名称", provider.Name)
			continue
		}
		providers[provider.Name] = provider.Config
	}
	if len(r.Providers) > 0 {
		override["rule-providers"] = providers
	}
	policies := map[string]bool{"DIRECT": true, "REJECT": true, "REJECT-DROP": true, "PASS": true, "COMPATIBLE": true, "GLOBAL": true}
	occupiedGroups := map[string]any{}
	proxyNames := []string{}
	for _, v := range values(effective["proxies"]) {
		name := identity(v)
		policies[name] = true
		proxyNames = append(proxyNames, name)
	}
	for _, v := range values(effective["proxy-groups"]) {
		name := identity(v)
		policies[name] = true
		occupiedGroups[name] = v
	}
	groupNames := map[string]bool{}
	for _, g := range r.Groups {
		name, _ := g["name"].(string)
		typ, _ := g["type"].(string)
		if name == "" || strings.ContainsAny(name, ",\r\n") || policies[name] && occupiedGroups[name] == nil || groupNames[name] {
			r.fail(0, "代理组名称无效、重复或与节点及内置策略冲突", name)
		}
		if typ != "select" && typ != "url-test" && typ != "fallback" && typ != "load-balance" {
			r.fail(0, "不支持的代理组类型", name)
		}
		groupNames[name] = true
		policies[name] = true
	}
	for _, g := range r.Groups {
		name, _ := g["name"].(string)
		if patterns, ok := g["_import_patterns"].([]string); ok {
			members, _ := g["proxies"].([]string)
			matched := false
			for _, pattern := range patterns {
				re := regexp2.MustCompile(pattern, regexp2.None)
				re.MatchTimeout = 10 * time.Millisecond
				for _, node := range proxyNames {
					matches, err := re.MatchString(node)
					if err != nil {
						r.fail(0, "代理组节点筛选超时", name)
						break
					}
					if matches {
						matched = true
					}
				}
			}
			if len(patterns) > 0 {
				if !matched && len(mapping(effective["proxy-providers"])) == 0 && len(members) == 0 {
					members = append(members, "DIRECT")
					r.warn(0, "代理组当前没有匹配节点，已加入 DIRECT；后续订阅更新仍会按筛选条件匹配", name)
				}
				filters := append([]string{}, patterns...)
				for _, member := range members {
					filters = append(filters, "^"+regexp.QuoteMeta(member)+"$")
				}
				g["include-all-proxies"] = true
				g["include-all-providers"] = true
				g["filter"] = strings.Join(filters, "|")
			}
			g["proxies"] = members
			delete(g, "_import_patterns")
		}
		if old, ok := occupiedGroups[name]; ok && !reflect.DeepEqual(cloneMap(mapping(old)), cloneMap(g)) {
			r.fail(0, "代理组名称与已有配置冲突，请修改源文件中的组名", name)
		}
		data, _ := json.Marshal(g)
		var normalized map[string]any
		_ = json.Unmarshal(data, &normalized)
		for _, key := range []string{"proxies", "use"} {
			if value, exists := normalized[key]; exists && value != nil {
				if _, ok := value.([]any); !ok {
					r.fail(0, "代理组成员与节点集合必须是列表", name+": "+key)
				}
			}
		}
		if len(values(normalized["proxies"])) == 0 && len(values(normalized["use"])) == 0 && normalized["include-all"] != true && normalized["include-all-proxies"] != true && normalized["include-all-providers"] != true {
			r.fail(0, "代理组没有节点或节点集合", name)
		}
		if filter, exists := normalized["filter"]; exists {
			pattern, ok := filter.(string)
			if _, err := regexp2.Compile(pattern, regexp2.None); !ok || err != nil {
				r.fail(0, "代理组节点筛选正则无效", name)
			}
		}
		for _, v := range values(normalized["proxies"]) {
			member, ok := v.(string)
			if !ok || !policies[member] || member == name {
				r.fail(0, "代理组引用不存在或引用自身", name+": "+fmt.Sprint(v))
			}
		}
		for _, v := range values(normalized["use"]) {
			member, _ := v.(string)
			if _, ok := mapping(effective["proxy-providers"])[member]; !ok {
				r.fail(0, "代理组引用的节点集合不存在", name+": "+member)
			}
		}
	}
	// Detect cycles across imported and existing groups.
	allGroups := map[string]any{}
	for name, g := range occupiedGroups {
		allGroups[name] = g
	}
	for _, g := range r.Groups {
		allGroups[identity(g)] = g
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(name string) bool {
		if visiting[name] {
			return false
		}
		if visited[name] {
			return true
		}
		visiting[name] = true
		data, _ := json.Marshal(allGroups[name])
		var g map[string]any
		_ = json.Unmarshal(data, &g)
		for _, v := range values(g["proxies"]) {
			member, _ := v.(string)
			if allGroups[member] != nil && !visit(member) {
				return false
			}
		}
		visiting[name] = false
		visited[name] = true
		return true
	}
	for _, g := range r.Groups {
		if !visit(identity(g)) {
			r.fail(0, "代理组存在循环引用", identity(g))
			break
		}
	}
	for _, rule := range r.Rules {
		parts := Parts(rule)
		if err := FullRule(rule); err != nil {
			r.fail(0, err.Error(), rule)
			continue
		}
		idx := 2
		if parts[0] == "MATCH" {
			idx = 1
		}
		if !policies[parts[idx]] {
			r.fail(0, "规则目标策略不存在，请选择已有策略或导入对应代理组", parts[idx])
		}
		if parts[0] == "RULE-SET" {
			if occupied[parts[1]] == nil && providers[parts[1]] == nil {
				r.fail(0, "规则引用的规则集不存在", parts[1])
			}
		}
	}
	rules, err := sequence(existing["rules"])
	if err != nil {
		return nil, err
	}
	groups, err := sequence(existing["groups"])
	if err != nil {
		return nil, err
	}
	// Respect INI's replacement default unless the user explicitly chooses merge.
	if o.Placement == "prepend" || o.Placement == "append" {
		r.Replace = false
	}
	if o.Placement == "replace" {
		r.Replace = true
	}
	ruleItems := make([]any, 0, len(r.Rules))
	for _, s := range r.Rules {
		ruleItems = append(ruleItems, s)
	}
	if r.Replace {
		rules.Prepend = ruleItems
		rules.Append = []any{}
		for _, v := range values(base["rules"]) {
			s, _ := v.(string)
			if s != "" && !stringIn(rules.Delete, s) {
				rules.Delete = append(rules.Delete, s)
			}
		}
		r.warn(0, "将替换本订阅原有规则和规则增强，原始订阅文件保持不变", "")
	}
	if _, shadows := override["rules"]; shadows {
		list := values(effective["rules"])
		if r.Replace {
			override["rules"] = ruleItems
		} else if o.Placement == "append" {
			override["rules"] = uniqueRules(append(list, ruleItems...))
		} else {
			override["rules"] = uniqueRules(append(ruleItems, list...))
		}
		r.warn(0, "此订阅已有 rules 覆写，导入规则将合并到该覆写列表", "")
	} else if !r.Replace {
		newItems := []any{}
		for _, v := range ruleItems {
			if !containsString(values(effective["rules"]), v.(string)) && !containsString(newItems, v.(string)) {
				newItems = append(newItems, v)
			}
		}
		if o.Placement == "append" {
			rules.Append = append(rules.Append, newItems...)
		} else {
			rules.Prepend = append(newItems, rules.Prepend...)
		}
	}
	if o.Placement == "append" {
		for _, v := range values(effective["rules"]) {
			if strings.HasPrefix(fmt.Sprint(v), "MATCH,") {
				r.warn(0, "前面已有 MATCH 兜底规则，后置导入规则可能无法匹配", "")
				break
			}
		}
	}
	for _, g := range r.Groups {
		if occupiedGroups[identity(g)] == nil {
			groups.Prepend = append(groups.Prepend, g)
		}
	}
	if len(r.Rules) > 0 {
		s, err := marshal(rules)
		if err != nil {
			return nil, err
		}
		p.Extensions["rules"] = s
	}
	if len(r.Groups) > 0 {
		s, err := marshal(groups)
		if err != nil {
			return nil, err
		}
		p.Extensions["groups"] = s
	}
	if len(r.Providers) > 0 || override["rules"] != nil && len(r.Rules) > 0 {
		s, err := marshal(override)
		if err != nil {
			return nil, err
		}
		p.Extensions["override"] = s
	}
	proposed := map[string]string{}
	for k, v := range existing {
		proposed[k] = v
	}
	for k, v := range p.Extensions {
		proposed[k] = v
	}
	finalRaw, err := configyaml.ApplyProfileExtensionChain(raw, profileName, global, proposed)
	if err != nil {
		r.fail(0, "合并增强配置失败", err.Error())
	} else {
		final, _ := object(finalRaw)
		for _, rule := range r.Rules {
			if !containsString(values(final["rules"]), rule) {
				r.fail(0, "全局覆写或脚本覆盖了导入规则，请先调整该增强", rule)
				break
			}
		}
		for _, provider := range r.Providers {
			if !reflect.DeepEqual(cloneMap(mapping(mapping(final["rule-providers"])[provider.Name])), cloneMap(provider.Config)) {
				r.fail(0, "覆写或脚本覆盖了导入规则集", provider.Name)
			}
		}
		for _, g := range r.Groups {
			found := false
			for _, v := range values(final["proxy-groups"]) {
				if identity(v) == identity(g) && reflect.DeepEqual(cloneMap(mapping(v)), cloneMap(g)) {
					found = true
				}
			}
			if !found {
				r.fail(0, "覆写或脚本覆盖了导入代理组", identity(g))
			}
		}
	}
	return p, nil
}
func stringIn(items []string, s string) bool {
	for _, v := range items {
		if v == s {
			return true
		}
	}
	return false
}
func uniqueRules(items []any) []any {
	out := []any{}
	seen := map[string]bool{}
	for _, v := range items {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
