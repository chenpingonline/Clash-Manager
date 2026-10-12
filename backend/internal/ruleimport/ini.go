package ruleimport

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

func parseINI(text string, o Options, fetch Fetch, r *Result) (*Result, error) {
	r.Format = "ACL4SSR INI"
	section := ""
	sources := map[string]string{}
	providerCount := 0
	enabled := true
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if section != "[custom]" {
			r.warn(i+1, "仅支持 INI 的 custom 段", line)
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			r.warn(i+1, "INI 条目缺少等号", line)
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "ruleset", "surge_ruleset":
			policy, source, ok := strings.Cut(value, ",")
			policy = strings.TrimSpace(policy)
			source = strings.TrimSpace(source)
			if !ok || policy == "" {
				r.fail(i+1, "ruleset 格式无效", line)
				continue
			}
			if strings.HasPrefix(source, "[]") {
				rule := strings.TrimPrefix(source, "[]")
				parts := Parts(rule)
				if parts[0] == "FINAL" || parts[0] == "MATCH" {
					r.Rules = append(r.Rules, "MATCH,"+policy)
					continue
				}
				// Insert policy before options, retaining no-resolve.
				suffix := ""
				if strings.HasSuffix(rule, ",no-resolve") {
					rule = strings.TrimSuffix(rule, ",no-resolve")
					suffix = ",no-resolve"
				}
				full := rule + "," + policy + suffix
				if err := FullRule(full); err != nil {
					r.warn(i+1, err.Error(), line)
				} else {
					r.Rules = append(r.Rules, full)
				}
				continue
			}
			b, f := "", ""
			for prefix, behavior := range map[string]string{"clash-domain:": "domain", "clash-ipcidr:": "ipcidr", "clash-classic:": "classical", "surge:": "classical"} {
				if strings.HasPrefix(source, prefix) {
					source = strings.TrimPrefix(source, prefix)
					b = behavior
					if prefix != "surge:" {
						f = "yaml"
					} else {
						f = "text"
					}
					break
				}
			}
			if strings.HasPrefix(source, "quanx:") {
				r.fail(i+1, "首批 INI 导入不支持 Quantumult X 规则源，请使用 Clash 或 Surge 格式", line)
				continue
			}
			interval := 86400
			if idx := strings.LastIndex(source, ","); idx >= 0 {
				n, err := strconv.Atoi(source[idx+1:])
				if err != nil || n < 60 {
					r.fail(i+1, "规则更新间隔必须是至少 60 秒的整数", line)
					continue
				}
				interval = n
				source = source[:idx]
			}
			u, err := URL(source)
			if err != nil {
				r.fail(i+1, "INI 中的规则源必须是 HTTP/HTTPS 链接；本地路径请单独上传", line)
				continue
			}
			name, exists := sources[u+"|"+b+"|"+f]
			if !exists {
				providerCount++
				if providerCount > 64 {
					return nil, errors.New("一次最多导入 64 个规则集")
				}
				name = fmt.Sprintf("%s_%02d", truncate(Name(o.Name), 64), providerCount)
				p, issues, err := remoteProvider(name, u, b, f, fetch)
				if err != nil {
					r.fail(i+1, err.Error(), line)
					continue
				}
				p.Config["interval"] = interval
				r.Providers = append(r.Providers, p)
				for _, issue := range issues {
					r.warn(i+1, issue.Message, issue.Content)
				}
				sources[u+"|"+b+"|"+f] = name
			}
			r.Rules = append(r.Rules, "RULE-SET,"+name+","+policy)
		case "custom_proxy_group":
			g, err := iniGroup(value)
			if err != nil {
				r.fail(i+1, err.Error(), line)
			} else {
				r.Groups = append(r.Groups, g)
			}
		case "overwrite_original_rules":
			v, err := strconv.ParseBool(value)
			if err != nil {
				r.fail(i+1, "overwrite_original_rules 必须为 true 或 false", line)
			} else {
				r.Replace = v
			}
		case "enable_rule_generator":
			v, err := strconv.ParseBool(value)
			if err != nil {
				r.fail(i+1, "enable_rule_generator 必须为 true 或 false", line)
			} else {
				enabled = v
			}
		default:
			r.warn(i+1, "此 INI 选项未导入", line)
		}
	}
	if !enabled {
		r.fail(0, "INI 已关闭规则生成，请修改 enable_rule_generator 后重新导入", "")
	}
	if len(r.Providers)+len(r.Rules)+len(r.Groups) == 0 {
		return nil, errors.New("INI 中没有可导入的规则或代理组")
	}
	return r, nil
}
func iniGroup(value string) (map[string]any, error) {
	fields := strings.Split(value, "`")
	if len(fields) < 3 {
		return nil, errors.New("代理组 INI 格式无效")
	}
	name, typ := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
	if name == "" || strings.ContainsAny(name, ",\r\n") {
		return nil, errors.New("代理组名称无效")
	}
	if typ != "select" && typ != "url-test" && typ != "fallback" && typ != "load-balance" {
		return nil, errors.New("不支持的 INI 代理组类型")
	}
	members := []string{}
	patterns := []string{}
	end := len(fields)
	g := map[string]any{"name": name, "type": typ}
	if typ != "select" {
		if len(fields) < 5 {
			return nil, errors.New("测速代理组缺少 URL 或间隔")
		}
		end -= 2
		u, err := URL(fields[end])
		if err != nil {
			return nil, errors.New("测速地址无效")
		}
		g["url"] = u
		timing := strings.Split(fields[end+1], ",")
		if len(timing) > 3 {
			return nil, errors.New("测速参数过多")
		}
		for i, s := range timing {
			if s == "" {
				continue
			}
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 || i == 0 && n == 0 {
				return nil, errors.New("测速间隔、超时或容差无效")
			}
			switch i {
			case 0:
				g["interval"] = n
			case 1:
				g["timeout"] = n * 1000
			case 2:
				g["tolerance"] = n
			}
		}
	}
	for _, field := range fields[2:end] {
		if strings.HasPrefix(field, "[]") {
			members = append(members, strings.TrimPrefix(field, "[]"))
		} else {
			if strings.Contains(field, "!!") || strings.HasPrefix(field, "script:") {
				return nil, errors.New("不支持 INI 的脚本、GROUPID 或特殊筛选条件")
			}
			if _, err := regexp2.Compile(field, regexp2.None); err != nil {
				return nil, errors.New("代理组节点筛选正则无效")
			}
			patterns = append(patterns, field)
		}
	}
	g["proxies"] = members
	// Retain the selectors as metadata until the planner resolves them against the
	// selected profile. They are never written into Mihomo YAML as private fields.
	g["_import_patterns"] = patterns
	return g, nil
}
