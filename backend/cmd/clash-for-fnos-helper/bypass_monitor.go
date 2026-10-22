package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type bypassInterface struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	CIDR    string `json:"cidr"`
}

func monitorInterfaces() []bypassInterface {
	out := []bypassInterface{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, network, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && ip.IsPrivate() {
				out = append(out, bypassInterface{iface.Name, ip.String(), network.String()})
			}
		}
	}
	return out
}

// This endpoint never creates a config, binds a port, changes sysctls, or
// modifies firewall rules. Missing evidence is returned as unknown.
func (h *helper) bypassMonitor(ctx context.Context) map[string]any {
	result := map[string]any{"interfaces": monitorInterfaces(), "config": nil, "forwarding": nil, "firewall": map[string]any{"readable": false}, "legacyJournal": fileExists(h.bypassStatePathForMonitor()), "neighbors": []bypassNeighbor{}, "neighborStatus": "unsupported"}
	path := h.config.managedConfig
	if proc := h.primary(); proc != nil && proc.ConfigPath != "" {
		path = proc.ConfigPath
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if config, e := monitorConfig(raw); e == nil {
			result["config"] = config
		} else {
			result["configError"] = "invalid-config"
		}
	} else {
		result["configError"] = "unavailable"
	}
	if runtime.GOOS != "linux" {
		return result
	}
	result["neighborStatus"] = "unavailable"
	if neighbors, e := readBypassNeighbors(ctx, result["interfaces"].([]bypassInterface)); e == nil {
		result["neighbors"] = neighbors
		result["neighborStatus"] = "available"
	}
	if raw, e := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); e == nil {
		switch strings.TrimSpace(string(raw)) {
		case "1":
			result["forwarding"] = true
		case "0":
			result["forwarding"] = false
		}
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, e := exec.CommandContext(readCtx, "nft", "--json", "list", "ruleset").Output()
	if e == nil {
		if firewall, err := summarizeFirewall(output); err == nil {
			result["firewall"] = firewall
		}
	}
	return result
}
func (h *helper) bypassStatePathForMonitor() string {
	return filepath.Join(h.config.etcDir, "bypass-runtime.json")
}
func monitorConfig(raw []byte) (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("configuration is not an object")
	}
	tun, _ := doc["tun"].(map[string]any)
	dns, _ := doc["dns"].(map[string]any)
	// Do not use UI defaults as evidence that a setting exists in the source.
	pick := func(m map[string]any, k string) any {
		if v, ok := m[k].(bool); ok {
			return v
		}
		return nil
	}
	listen, _ := dns["listen"].(string)
	excluded := []string{}
	if list, ok := tun["route-exclude-address"].([]any); ok {
		for _, v := range list {
			if text, ok := v.(string); ok {
				excluded = append(excluded, text)
			}
		}
	}
	hijack := []string{}
	if list, ok := tun["dns-hijack"].([]any); ok {
		for _, v := range list {
			if text, ok := v.(string); ok {
				hijack = append(hijack, text)
			}
		}
	}
	return map[string]any{"tun": pick(tun, "enable"), "autoRoute": pick(tun, "auto-route"), "autoRedirect": pick(tun, "auto-redirect"), "dnsEnabled": pick(dns, "enable"), "dnsListen": listen, "dnsHijack": hijack, "routeExclude": excluded}, nil
}
func summarizeFirewall(raw []byte) (map[string]any, error) {
	var document struct {
		Items []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if document.Items == nil {
		return nil, fmt.Errorf("nftables ruleset unavailable")
	}
	forward, nat, drops := 0, 0, 0
	for _, item := range document.Items {
		if body, ok := item["chain"]; ok {
			var chain struct {
				Hook string `json:"hook"`
			}
			if json.Unmarshal(body, &chain) == nil && chain.Hook == "forward" {
				forward++
			}
		}
		if body, ok := item["rule"]; ok {
			var rule struct {
				Expr []map[string]json.RawMessage `json:"expr"`
			}
			if json.Unmarshal(body, &rule) == nil {
				for _, expr := range rule.Expr {
					if _, ok := expr["masquerade"]; ok {
						nat++
					}
					if _, ok := expr["snat"]; ok {
						nat++
					}
					if _, ok := expr["drop"]; ok {
						drops++
					}
				}
			}
		}
	}
	return map[string]any{"readable": true, "forwardChains": forward, "natRules": nat, "dropRules": drops}, nil
}
