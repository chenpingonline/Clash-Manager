package configyaml

import (
	"net"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnsureLANBindAddress widens a loopback-only proxy binding when LAN access is
// enabled. Custom interfaces, wildcard/default bindings and disabled LAN access
// retain their configuration. Apply this after subscription and user overrides.
func EnsureLANBindAddress(raw []byte) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	enabled, _ := config["allow-lan"].(bool)
	bind, _ := config["bind-address"].(string)
	if !enabled || !isLoopbackBindAddress(bind) {
		return raw, nil
	}
	return MergeOverrides(raw, map[string]any{"bind-address": "*"})
}

func isLoopbackBindAddress(address string) bool {
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		address = address[1 : len(address)-1]
	}
	if strings.EqualFold(address, "localhost") {
		return true
	}
	if ip := net.ParseIP(address); ip != nil {
		return ip.IsLoopback()
	}
	// Also recognize traditional IPv4 shorthand such as 127.1 and 127.0.1.
	parts := strings.Split(address, ".")
	if len(parts) < 2 || len(parts) > 4 || parts[0] != "127" {
		return false
	}
	for i, part := range parts[1:] {
		value, err := strconv.ParseUint(part, 10, 32)
		limit := uint64(255)
		if i == len(parts)-2 {
			limit = (uint64(1) << (8 * (5 - len(parts)))) - 1
		}
		if err != nil || value > limit {
			return false
		}
	}
	return true
}
