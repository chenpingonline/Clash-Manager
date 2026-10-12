package configyaml

import (
	"fmt"
	"net/netip"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProxyAccount struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ProxyAuthentication describes the top-level HTTP/SOCKS/Mixed credentials.
// Listener-specific users are deliberately left in their original configuration.
type ProxyAuthentication struct {
	Accounts         []ProxyAccount `json:"accounts"`
	SkipAuthPrefixes []string       `json:"skipAuthPrefixes"`
}

func ProxyAuthenticationFromDocument(document map[string]any) (ProxyAuthentication, error) {
	result := ProxyAuthentication{Accounts: []ProxyAccount{}, SkipAuthPrefixes: []string{}}
	for _, key := range []string{"authentication", "skip-auth-prefixes"} {
		if document[key] == nil {
			continue
		}
		items, ok := document[key].([]any)
		if !ok {
			return result, fmt.Errorf("%s 必须为字符串列表", key)
		}
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return result, fmt.Errorf("%s 必须为字符串列表", key)
			}
			if key == "authentication" {
				// Mihomo splits at the first colon, so passwords may contain colons.
				username, password, found := strings.Cut(text, ":")
				if !found {
					return result, fmt.Errorf("代理认证记录必须使用用户名:密码格式")
				}
				result.Accounts = append(result.Accounts, ProxyAccount{username, password})
			} else {
				result.SkipAuthPrefixes = append(result.SkipAuthPrefixes, text)
			}
		}
	}
	return result, nil
}

func ParseProxyAuthentication(raw []byte) (ProxyAuthentication, error) {
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return ProxyAuthentication{}, err
	}
	return ProxyAuthenticationFromDocument(document)
}

func (auth ProxyAuthentication) Overrides() (map[string]any, error) {
	records, prefixes := []string{}, []string{}
	seen := map[string]bool{}
	for _, account := range auth.Accounts {
		if strings.TrimSpace(account.Username) == "" || account.Password == "" {
			return nil, fmt.Errorf("代理认证的用户名和密码不能为空")
		}
		if strings.ContainsAny(account.Username, ":\r\n\x00") || strings.ContainsAny(account.Password, "\r\n\x00") {
			return nil, fmt.Errorf("代理认证的用户名不能包含冒号，账号不能包含换行或空字符")
		}
		if seen[account.Username] {
			return nil, fmt.Errorf("代理认证的用户名不能重复")
		}
		seen[account.Username] = true
		records = append(records, account.Username+":"+account.Password)
	}
	for _, value := range auth.SkipAuthPrefixes {
		value = strings.TrimSpace(value)
		if _, err := netip.ParsePrefix(value); err != nil {
			return nil, fmt.Errorf("免认证网段必须为有效的 IPv4 或 IPv6 CIDR")
		}
		prefixes = append(prefixes, value)
	}
	return map[string]any{"authentication": records, "skip-auth-prefixes": prefixes}, nil
}
