package configyaml

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Independent listeners are preferences, not a replacement for subscription listeners.
// The YAML comment records ownership without adding non-Mihomo fields to the config.
type IndependentListener struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Listen  string         `json:"listen"`
	Port    int            `json:"port"`
	Proxy   string         `json:"proxy"`
	Enabled bool           `json:"enabled"`
	Users   []ProxyAccount `json:"users"`
}

const IndependentListenersKey = "manager-independent-listeners"
const listenerMarker = "clash-manager-independent:"

var listenerID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

func ValidateIndependentListeners(entries []IndependentListener) error {
	if len(entries) > 64 {
		return fmt.Errorf("独立代理入口最多 64 个")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		if !listenerID.MatchString(e.ID) || ids[e.ID] {
			return fmt.Errorf("独立代理入口 ID 无效或重复")
		}
		ids[e.ID] = true
		if strings.TrimSpace(e.Name) == "" || len(e.Name) > 128 || strings.ContainsAny(e.Name, "\r\n\x00") || names[e.Name] {
			return fmt.Errorf("独立代理入口名称为空、过长或重复")
		}
		names[e.Name] = true
		if e.Type != "mixed" && e.Type != "socks" && e.Type != "http" {
			return fmt.Errorf("独立代理入口协议无效")
		}
		if net.ParseIP(e.Listen) == nil {
			return fmt.Errorf("独立代理入口监听地址必须为 IP 地址")
		}
		if e.Port < 1 || e.Port > 65535 {
			return fmt.Errorf("独立代理入口端口必须为 1–65535")
		}
		if strings.TrimSpace(e.Proxy) == "" || len(e.Proxy) > 256 {
			return fmt.Errorf("独立代理入口必须指定目标节点或策略组")
		}
		if len(e.Users) > 16 {
			return fmt.Errorf("独立代理入口认证账户最多 16 个")
		}
		seen := map[string]bool{}
		for _, u := range e.Users {
			if strings.TrimSpace(u.Username) == "" || u.Password == "" || len(u.Username) > 128 || len(u.Password) > 256 || strings.ContainsAny(u.Username+u.Password, "\r\n\x00") || seen[u.Username] {
				return fmt.Errorf("独立代理入口认证账户无效或重复")
			}
			seen[u.Username] = true
		}
	}
	return nil
}

func IndependentListenerTargets(raw []byte) []string {
	var config struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
		Groups []struct {
			Name string `yaml:"name"`
		} `yaml:"proxy-groups"`
	}
	if yaml.Unmarshal(raw, &config) != nil {
		return []string{}
	}
	names := []string{"DIRECT", "REJECT"}
	seen := map[string]bool{"DIRECT": true, "REJECT": true}
	for _, p := range config.Proxies {
		if p.Name != "" && !seen[p.Name] {
			names = append(names, p.Name)
			seen[p.Name] = true
		}
	}
	for _, p := range config.Groups {
		if p.Name != "" && !seen[p.Name] {
			names = append(names, p.Name)
			seen[p.Name] = true
		}
	}
	return names
}

// MergeIndependentListeners preserves unowned nodes and comments. A missing target
// stops only that managed entry, retaining the preference for automatic recovery.
func MergeIndependentListeners(raw []byte, entries, previous []IndependentListener) ([]byte, []string, error) {
	if err := ValidateIndependentListeners(entries); err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("配置必须为 YAML 对象")
	}
	root := doc.Content[0]
	index := -1
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "listeners" {
			index = i
			break
		}
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if index >= 0 {
		source := root.Content[index+1]
		// Keep anchors defined on listeners valid for aliases elsewhere. An
		// alias-based listeners section keeps its original anchor at its source.
		seq.Anchor = source.Anchor
		if source.Kind == yaml.AliasNode {
			source = source.Alias
		}
		if source.Kind != yaml.SequenceNode && source.Tag != "!!null" {
			return nil, nil, fmt.Errorf("listeners 必须为列表")
		}
		seq.HeadComment = source.HeadComment
		seq.LineComment = source.LineComment
		seq.FootComment = source.FootComment
		old := map[string]bool{}
		for _, e := range previous {
			old[e.ID] = true
		}
		for _, node := range source.Content {
			owned := false
			for _, line := range strings.Split(node.HeadComment, "\n") {
				marker := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
				if strings.HasPrefix(marker, listenerMarker) && old[strings.TrimPrefix(marker, listenerMarker)] {
					owned = true
				}
			}
			if !owned {
				seq.Content = append(seq.Content, node)
			}
		}
	}
	names := map[string]bool{}
	for _, node := range seq.Content {
		var e struct {
			Name string `yaml:"name"`
		}
		if err := node.Decode(&e); err != nil {
			return nil, nil, err
		}
		names[e.Name] = true
	}
	targets := map[string]bool{}
	for _, name := range IndependentListenerTargets(raw) {
		targets[name] = true
	}
	missing := []string{}
	for _, e := range entries {
		if names[e.Name] {
			return nil, nil, fmt.Errorf("独立代理入口 %s 与订阅 listeners 名称冲突", e.Name)
		}
		names[e.Name] = true
		if !e.Enabled {
			continue
		}
		if !targets[e.Proxy] {
			missing = append(missing, e.ID)
			continue
		}
		value := map[string]any{"name": e.Name, "type": e.Type, "listen": e.Listen, "port": e.Port, "proxy": e.Proxy, "udp": e.Type != "http"}
		// An explicit empty users list opts out of global authentication. Never
		// implicitly inherit credentials or bypass rules from the shared proxy port.
		value["users"] = e.Users
		if e.Users == nil {
			value["users"] = []ProxyAccount{}
		}
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return nil, nil, err
		}
		node.HeadComment = listenerMarker + e.ID
		seq.Content = append(seq.Content, &node)
	}
	if index >= 0 {
		root.Content[index+1] = seq
	} else if len(seq.Content) > 0 {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "listeners"}, seq)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), missing, enc.Close()
}
