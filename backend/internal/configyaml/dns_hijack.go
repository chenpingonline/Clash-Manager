package configyaml

import (
	"bytes"
	"fmt"
	"net/netip"
	"strings"

	"gopkg.in/yaml.v3"
)

// NormalizeTunDNSHijack fills the default DNS port before runtime application.
// Mihomo -t does not validate these endpoints; the TUN listener does so later.
func NormalizeTunDNSHijack(raw []byte) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return raw, nil
	}
	root := document.Content[0]
	var config map[string]any
	if err := root.Decode(&config); err != nil {
		return nil, err
	}
	if dns, ok := config["dns"].(map[string]any); ok {
		if _, exists := dns["dns-hijack"]; exists {
			return nil, fmt.Errorf("DNS 劫持配置位置错误：dns-hijack 应放在 tun 下，而不是 dns 下；DNS 上游服务器请填写 nameserver")
		}
	}
	tun, _ := config["tun"].(map[string]any)
	value, exists := tun["dns-hijack"]
	if !exists || value == nil {
		return raw, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("tun.dns-hijack 必须是地址字符串列表，例如 [\"any:53\", \"tcp://any:53\"]")
	}
	normalized := make([]string, len(items))
	changed := false
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("tun.dns-hijack 第 %d 项必须是地址字符串，例如 114.114.114.114:53", i+1)
		}
		endpoint, err := normalizeDNSHijackEndpoint(text)
		if err != nil {
			return nil, fmt.Errorf("tun.dns-hijack 第 %d 项格式无效：%w", i+1, err)
		}
		normalized[i] = endpoint
		changed = changed || endpoint != text
	}
	if !changed {
		return raw, nil
	}
	// Edit only this sequence. Keep rules, provider order and unrelated comments.
	tunNode := mappingValue(root, "tun")
	if tunNode == nil || tunNode.Kind != yaml.MappingNode {
		tunNode = &yaml.Node{}
		if err := tunNode.Encode(tun); err != nil {
			return nil, err
		}
		setMappingValue(root, "tun", tunNode)
	}
	previous := mappingValue(tunNode, "dns-hijack")
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if previous != nil {
		*sequence = *previous
		if sequence.Kind == yaml.AliasNode {
			*sequence = *sequence.Alias
			sequence.Anchor = ""
		}
		sequence.Kind, sequence.Tag, sequence.Alias = yaml.SequenceNode, "!!seq", nil
	}
	sequence.Content = make([]*yaml.Node, len(normalized))
	for i, text := range normalized {
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text}
		if previous != nil && previous.Kind == yaml.SequenceNode && i < len(previous.Content) {
			*node = *previous.Content[i]
			// Materialize scalar aliases instead of changing other references to them.
			if node.Kind == yaml.AliasNode {
				*node = *node.Alias
				node.Anchor = ""
			}
			node.Kind, node.Tag, node.Value, node.Alias = yaml.ScalarNode, "!!str", text, nil
		}
		sequence.Content[i] = node
	}
	setMappingValue(tunNode, "dns-hijack", sequence)
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	return output.Bytes(), encoder.Close()
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
func setMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func normalizeDNSHijackEndpoint(input string) (string, error) {
	endpoint := strings.TrimSpace(input)
	prefix := ""
	if scheme, address, found := strings.Cut(endpoint, "://"); found {
		if scheme != "udp" && scheme != "tcp" {
			return "", fmt.Errorf("仅支持 udp:// 或 tcp:// 协议，请使用 udp://IP:端口 或 tcp://IP:端口")
		}
		prefix, endpoint = scheme+"://", address
	}
	// Parse the whole bare IPv6 address before considering a port suffix.
	// Explicit IPv6 ports must use brackets, e.g. [2001:db8::1]:53.
	bare := endpoint
	if strings.HasPrefix(bare, "[") && strings.HasSuffix(bare, "]") {
		bare = bare[1 : len(bare)-1]
	}
	if address, err := netip.ParseAddr(bare); err == nil {
		return prefix + netip.AddrPortFrom(address, 53).String(), nil
	}
	parsed := endpoint
	if strings.HasPrefix(parsed, "any:") {
		parsed = "0.0.0.0:" + strings.TrimPrefix(parsed, "any:")
	}
	address, err := netip.ParseAddrPort(parsed)
	if err != nil || address.Port() == 0 {
		return "", fmt.Errorf("请填写有效的 IP:端口 或 any:端口，端口范围为 1–65535；IPv6 请用 [IPv6]:端口，例如 [2001:db8::1]:53；纯 IP 会自动补上 :53")
	}
	return prefix + endpoint, nil
}
