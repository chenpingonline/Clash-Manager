package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type networkListener struct{ network, address, label string }

func networkListenersOverlap(left, right networkListener) bool {
	if left.network != right.network {
		return false
	}
	leftHost, leftPort, leftErr := net.SplitHostPort(left.address)
	rightHost, rightPort, rightErr := net.SplitHostPort(right.address)
	if leftErr != nil || rightErr != nil || leftPort != rightPort {
		return false
	}
	leftIP, rightIP := net.ParseIP(leftHost), net.ParseIP(rightHost)
	if leftIP == nil || rightIP == nil {
		return leftHost == rightHost
	}
	leftIPv4, rightIPv4 := leftIP.To4() != nil, rightIP.To4() != nil
	if leftIPv4 != rightIPv4 {
		return (!leftIPv4 && leftIP.IsUnspecified()) || (!rightIPv4 && rightIP.IsUnspecified())
	}
	return leftIP.Equal(rightIP) || leftIP.IsUnspecified() || rightIP.IsUnspecified()
}

func networkListeners(raw string) []networkListener {
	controller, _, _ := parseController(raw)
	result := []networkListener{{"tcp", controller, "Controller API"}}
	host := "127.0.0.1"
	if yamlBoolean(raw, "allow-lan", false) {
		host = "0.0.0.0"
		if bind, ok := yamlScalarValue(raw, "bind-address"); ok && bind != "" && bind != "*" {
			host = strings.TrimSuffix(strings.TrimPrefix(bind, "["), "]")
		}
	}
	for _, item := range []struct {
		key, label string
		udp        bool
	}{
		{"mixed-port", "混合代理端口", true}, {"port", "HTTP(S) 代理端口", false}, {"socks-port", "SOCKS5 代理端口", true}, {"redir-port", "Redir 透明代理端口", false}, {"tproxy-port", "TProxy 透明代理端口", true},
	} {
		value, _ := yamlScalarValue(raw, item.key)
		port, _ := strconv.Atoi(value)
		if port <= 0 {
			continue
		}
		address := net.JoinHostPort(host, strconv.Itoa(port))
		result = append(result, networkListener{"tcp", address, item.label})
		if item.udp {
			result = append(result, networkListener{"udp", address, item.label})
		}
	}
	return result
}
func validateNetworkPorts(raw, previous string, offline bool) error {
	next := networkListeners(raw)
	extra, err := customNetworkListeners(raw)
	if err != nil {
		return err
	}
	next = append(next, extra...)
	// Reject duplicate TCP/UDP ports before probing, including currently owned ports.
	for i, item := range next {
		if _, _, err := net.SplitHostPort(item.address); err != nil {
			return fmt.Errorf("%s 监听地址无效: %w", item.label, err)
		}
		for _, prior := range next[:i] {
			if networkListenersOverlap(prior, item) {
				return fmt.Errorf("端口冲突：%s 已用于 %s，%s 请更换端口", item.address, prior.label, item.label)
			}
		}
	}
	old := []networkListener{}
	if !offline {
		old = networkListeners(previous)
		previousExtra, err := customNetworkListeners(previous)
		if err != nil {
			return err
		}
		old = append(old, previousExtra...)
	}
	for _, item := range next {
		// The running Core releases its listeners while applying the candidate.
		// Treat a wildcard/specific-address transition on the same port as the
		// current Core's listener instead of probing it against itself.
		ownedByCurrentCore := false
		for _, current := range old {
			if networkListenersOverlap(current, item) {
				ownedByCurrentCore = true
				break
			}
		}
		if ownedByCurrentCore {
			continue
		}
		var err error
		if item.network == "udp" {
			var conn net.PacketConn
			conn, err = net.ListenPacket(item.network, item.address)
			if err == nil {
				conn.Close()
			}
		} else {
			var conn net.Listener
			conn, err = net.Listen(item.network, item.address)
			if err == nil {
				conn.Close()
			}
		}
		if err != nil {
			return fmt.Errorf("%s 无法使用 %s (%s)，请更换端口或释放占用后重试: %w", item.label, item.address, item.network, err)
		}
	}
	return nil
}

// Parse actual listeners as YAML, including inline sequences and port ranges.
func customNetworkListeners(raw string) ([]networkListener, error) {
	var config struct {
		Listeners []struct {
			Name   string `yaml:"name"`
			Type   string `yaml:"type"`
			Listen string `yaml:"listen"`
			Port   any    `yaml:"port"`
			UDP    bool   `yaml:"udp"`
		} `yaml:"listeners"`
	}
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	result := []networkListener{}
	for _, e := range config.Listeners {
		if e.Port == nil {
			continue
		}
		host := e.Listen
		if host == "" {
			host = "0.0.0.0"
		}
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		if net.ParseIP(host) == nil {
			return nil, fmt.Errorf("listener %s 监听地址必须为 IP 地址", e.Name)
		}
		for _, part := range strings.Split(fmt.Sprint(e.Port), ",") {
			bounds := strings.Split(strings.TrimSpace(part), "-")
			first, err := strconv.Atoi(bounds[0])
			if err != nil || first < 1 || first > 65535 {
				return nil, fmt.Errorf("listener %s 端口无效", e.Name)
			}
			last := first
			if len(bounds) > 2 {
				return nil, fmt.Errorf("listener %s 端口范围无效", e.Name)
			}
			if len(bounds) == 2 {
				last, err = strconv.Atoi(bounds[1])
				if err != nil || last < first || last > 65535 {
					return nil, fmt.Errorf("listener %s 端口范围无效", e.Name)
				}
			}
			if len(result)+(last-first+1)*2 > 4096 {
				return nil, fmt.Errorf("listeners 端口范围过大，无法预检")
			}
			for port := first; port <= last; port++ {
				address := net.JoinHostPort(host, strconv.Itoa(port))
				udpOnly := e.Type == "tuic" || e.Type == "hysteria" || e.Type == "hysteria2"
				if !udpOnly {
					result = append(result, networkListener{"tcp", address, e.Name})
				}
				if udpOnly || e.UDP || e.Type == "tproxy" || e.Type == "shadowsocks" {
					result = append(result, networkListener{"udp", address, e.Name})
				}
			}
		}
	}
	return result, nil
}
