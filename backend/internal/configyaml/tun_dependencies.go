package configyaml

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"net"
)

type TunRequirements struct {
	DNSEnabled    bool     `json:"dnsEnabled"`
	HijackEnabled bool     `json:"hijackEnabled"`
	FakeIP        bool     `json:"fakeIP"`
	Issues        []string `json:"issues"`
	Warnings      []string `json:"warnings"`
}

func TunDNSRequirements(raw []byte) (TunRequirements, error) {
	r := TunRequirements{Issues: []string{}, Warnings: []string{}}
	var doc struct {
		Tun struct {
			Enable bool     `yaml:"enable"`
			Hijack []string `yaml:"dns-hijack"`
		} `yaml:"tun"`
		DNS struct {
			Enable bool   `yaml:"enable"`
			Mode   string `yaml:"enhanced-mode"`
			Range  string `yaml:"fake-ip-range"`
			Range6 string `yaml:"fake-ip-range6"`
		} `yaml:"dns"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return r, err
	}
	r.DNSEnabled = doc.DNS.Enable
	r.HijackEnabled = len(doc.Tun.Hijack) > 0
	r.FakeIP = doc.DNS.Mode == "fake-ip"
	if r.HijackEnabled && !r.DNSEnabled {
		r.Issues = append(r.Issues, "DNS 劫持需要先启用 Mihomo DNS，请在 DNS 设置中启用并应用，或关闭 DNS 劫持后重试")
	}
	if r.DNSEnabled && r.FakeIP {
		if doc.DNS.Range == "" {
			r.Warnings = append(r.Warnings, "Fake-IP 未配置 IPv4 地址范围，将沿用内核默认值")
		}
		for _, value := range []string{doc.DNS.Range, doc.DNS.Range6} {
			if value != "" {
				if _, _, err := net.ParseCIDR(value); err != nil {
					r.Issues = append(r.Issues, "Fake-IP 地址范围必须为有效的 CIDR")
				}
			}
		}
	}
	if doc.Tun.Enable && len(r.Issues) > 0 {
		return r, fmt.Errorf("%s", r.Issues[0])
	}
	return r, nil
}

func TunEnabled(raw []byte) bool {
	var doc struct {
		Tun struct {
			Enable bool `yaml:"enable"`
		} `yaml:"tun"`
	}
	return yaml.Unmarshal(raw, &doc) == nil && doc.Tun.Enable
}
