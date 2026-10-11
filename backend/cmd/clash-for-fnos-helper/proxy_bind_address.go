package main

import (
	"net"
	"strings"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
)

func proxyBindAddressFromConfig(raw string) string {
	if address, ok := yamlScalarValue(raw, "bind-address"); ok && address != "" {
		return address
	}
	return "*"
}

func normalizeProxyBindAddress(value any) (string, error) {
	address, ok := value.(string)
	if !ok {
		return "", fail(400, "代理监听地址必须为 IP 地址或 *")
	}
	address = strings.TrimSpace(address)
	if address == "*" {
		return address, nil
	}
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		address = address[1 : len(address)-1]
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return "", fail(400, "代理监听地址必须为 IP 地址或 *")
	}
	if ip.To4() == nil {
		return "[" + ip.String() + "]", nil
	}
	return ip.String(), nil
}

// Preserve addresses explicitly saved in the listening settings. Subscription
// loopback defaults still widen when enabling LAN without a user preference.
func ensureLANBindWithPreferences(raw []byte, settings map[string]any) ([]byte, error) {
	if _, explicit := settings["bind-address"]; explicit {
		return raw, nil
	}
	return configyaml.EnsureLANBindAddress(raw)
}
