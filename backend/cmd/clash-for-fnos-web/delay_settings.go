package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
	"gopkg.in/yaml.v3"
)

type groupDelaySetting struct {
	Mode string `json:"mode"`
	URL  string `json:"url,omitempty"`
}
type groupDelayTarget struct {
	Mode      string `json:"mode"`
	URL       string `json:"url"`
	ConfigURL string `json:"configUrl,omitempty"`
	Expected  string `json:"expected,omitempty"`
	Source    string `json:"source"`
}
type groupDelayContext struct {
	Scope     string                      `json:"scope"`
	GlobalURL string                      `json:"globalUrl"`
	Groups    map[string]groupDelayTarget `json:"groups"`
}

func validDelayURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil && len(value) <= 4096
}

// Callers hold configMu, shared with configuration activation. Scope never
// contains subscription URLs or credentials, and survives profile updates.
func (g *gateway) groupDelayScope(client *mihomo.Client) (string, error) {
	settings, err := client.LoadSettings()
	if err != nil {
		return "", err
	}
	scope, err := g.currentRuleScope()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(settings.Controller+"\n"+scope))), nil
}

func delaySettingsFor(document map[string]any, scope string) map[string]groupDelaySetting {
	var scopes map[string]map[string]groupDelaySetting
	body, _ := json.Marshal(document["groupDelaySettings"])
	_ = json.Unmarshal(body, &scopes)
	if scopes[scope] == nil {
		return map[string]groupDelaySetting{}
	}
	return scopes[scope]
}

func (g *gateway) groupDelayContext(ctx context.Context, client *mihomo.Client, payload map[string]any) (groupDelayContext, error) {
	scope, err := g.groupDelayScope(client)
	if err != nil {
		return groupDelayContext{}, err
	}
	settings, err := client.LoadSettings()
	if err != nil {
		return groupDelayContext{}, err
	}
	document, err := g.settings.ReadDocument()
	if err != nil {
		return groupDelayContext{}, err
	}
	overrides := delaySettingsFor(document, scope)
	result := groupDelayContext{Scope: scope, GlobalURL: settings.HealthcheckURL, Groups: map[string]groupDelayTarget{}}
	// Reading the effective YAML distinguishes explicit group URLs from the
	// Core's implicit default testUrl, and supports older Core API responses.
	var config struct {
		Groups []struct {
			Name     string `yaml:"name"`
			URL      string `yaml:"url"`
			Expected string `yaml:"expected-status"`
		} `yaml:"proxy-groups"`
	}
	raw, _, rawErr := g.activeStartupConfig(ctx)
	hasConfig := rawErr == nil && yaml.Unmarshal(raw, &config) == nil && len(raw) > 0
	configURLs, expected := map[string]string{}, map[string]string{}
	if hasConfig {
		for _, group := range config.Groups {
			configURLs[group.Name] = group.URL
			expected[group.Name] = group.Expected
		}
	}
	proxies, _ := payload["proxies"].(map[string]any)
	for name, entry := range proxies {
		proxy, ok := entry.(map[string]any)
		if !ok || proxy["all"] == nil {
			continue
		}
		configURL, _ := proxy["testUrl"].(string)
		expectation, _ := proxy["expectedStatus"].(string)
		if hasConfig {
			configURL = configURLs[name]
			expectation = expected[name]
		}
		configURL = strings.TrimSpace(configURL)
		if !validDelayURL(configURL) {
			configURL = ""
		}
		setting := overrides[name]
		if setting.Mode != "custom" && setting.Mode != "global" {
			setting.Mode = "auto"
		}
		target := groupDelayTarget{Mode: setting.Mode, ConfigURL: configURL, URL: settings.HealthcheckURL, Source: "global"}
		if setting.Mode == "custom" && validDelayURL(setting.URL) {
			target.URL = setting.URL
			target.Source = "custom"
		} else if setting.Mode == "auto" && configURL != "" {
			target.URL = configURL
			target.Expected = expectation
			target.Source = "config"
		}
		result.Groups[name] = target
	}
	return result, nil
}

func (g *gateway) saveGroupDelaySetting(w http.ResponseWriter, r *http.Request, client *mihomo.Client) {
	body, ok := readLimitedBody(w, r, 1<<20)
	if !ok {
		return
	}
	var request struct {
		Scope string `json:"scope"`
		Group string `json:"group"`
		groupDelaySetting
	}
	if json.Unmarshal(body, &request) != nil {
		writeJSON(w, 400, map[string]string{"error": "测速请求格式错误"})
		return
	}
	request.URL = strings.TrimSpace(request.URL)
	if request.Mode != "auto" && request.Mode != "global" && request.Mode != "custom" {
		writeJSON(w, 400, map[string]string{"error": "测速地址模式无效"})
		return
	}
	if request.Mode == "custom" && !validDelayURL(request.URL) {
		writeJSON(w, 400, map[string]string{"error": "测速地址只支持有效的 http/https URL"})
		return
	}
	g.configMu.Lock()
	defer g.configMu.Unlock()
	payload, err := mihomoJSON(r.Context(), client, "/proxies")
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	context, err := g.groupDelayContext(r.Context(), client, payload)
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	if request.Scope == "" || request.Scope != context.Scope {
		writeJSON(w, 409, map[string]string{"error": "当前配置已切换，请刷新后重试"})
		return
	}
	if _, ok := context.Groups[request.Group]; !ok {
		writeJSON(w, 400, map[string]string{"error": "代理组不存在"})
		return
	}
	err = g.settings.WithDocument(func(document map[string]any) error {
		var scopes map[string]map[string]groupDelaySetting
		raw, _ := json.Marshal(document["groupDelaySettings"])
		if err := json.Unmarshal(raw, &scopes); err != nil && string(raw) != "null" {
			return errors.New("读取组测速设置失败")
		}
		if scopes == nil {
			scopes = map[string]map[string]groupDelaySetting{}
		}
		if scopes[context.Scope] == nil {
			scopes[context.Scope] = map[string]groupDelaySetting{}
		}
		if request.Mode == "auto" {
			delete(scopes[context.Scope], request.Group)
		} else {
			setting := request.groupDelaySetting
			if setting.Mode != "custom" {
				setting.URL = ""
			}
			scopes[context.Scope][request.Group] = setting
		}
		document["groupDelaySettings"] = scopes
		return nil
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	context, err = g.groupDelayContext(r.Context(), client, payload)
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	writeJSON(w, 200, context)
}
