package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

const maxExitLocationResponse = 128 << 10

type exitLocationPayload struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Region      string `json:"region"`
	City        string `json:"city"`
	Timezone    struct {
		ID  string `json:"id"`
		UTC string `json:"utc"`
	} `json:"timezone"`
}

func runtimePort(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func runtimeProxy(configs map[string]any) (*url.URL, string) {
	for _, candidate := range []struct {
		key, scheme, via string
	}{
		{key: "mixed-port", scheme: "http", via: "mixed"},
		{key: "port", scheme: "http", via: "http"},
		{key: "socks-port", scheme: "socks5", via: "socks5"},
	} {
		if port := runtimePort(configs[candidate.key]); port > 0 && port <= 65535 {
			proxyURL, _ := url.Parse(fmt.Sprintf("%s://127.0.0.1:%d", candidate.scheme, port))
			return proxyURL, candidate.via
		}
	}
	return nil, "direct"
}

func (g *gateway) writeExitLocation(w http.ResponseWriter, r *http.Request, mihomoClient *mihomo.Client) {
	configs, err := mihomoJSON(r.Context(), mihomoClient, "/configs")
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	transport.Proxy = nil // A direct fallback must not inherit stale system proxy variables.
	proxyURL, via := runtimeProxy(configs)
	if proxyURL != nil {
		// /configs does not expose Mihomo's credentials. Read the owning active
		// startup YAML through Helper, including hand-written authentication.
		raw, _, readErr := g.activeStartupConfig(r.Context())
		if readErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "读取代理认证配置失败，请检查 Helper 与当前启动配置"})
			return
		}
		auth, parseErr := configyaml.ParseProxyAuthentication(raw)
		if parseErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "代理认证配置格式无效"})
			return
		}
		if len(auth.Accounts) > 0 {
			proxyURL.User = url.UserPassword(auth.Accounts[0].Username, auth.Accounts[0].Password)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	locationClient := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, g.config.exitLocationURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "出口位置服务配置无效"})
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Clash-for-fnOS/"+version)
	response, err := locationClient.Do(request)
	if err != nil {
		message := "出口位置更新失败: " + err.Error()
		if proxyURL != nil && proxyURL.User != nil {
			// net/http errors can contain a proxy URL; never return credentials.
			message = "出口位置更新失败，无法通过已配置的认证代理连接"
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": message})
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusProxyAuthRequired {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "代理认证失败，请检查用户名、密码与免认证网段"})
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("出口位置更新失败: HTTP %d", response.StatusCode)})
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxExitLocationResponse+1))
	if err != nil || len(body) > maxExitLocationResponse {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出口位置响应读取失败"})
		return
	}
	var payload exitLocationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出口位置服务返回了无效数据"})
		return
	}
	if !payload.Success || strings.TrimSpace(payload.IP) == "" {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = "未返回出口 IP"
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出口位置更新失败: " + message})
		return
	}
	if net.ParseIP(payload.IP) == nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出口位置服务返回了无效 IP"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ip": payload.IP, "country": payload.Country, "countryCode": strings.ToUpper(payload.CountryCode),
		"region": payload.Region, "city": payload.City, "timezone": payload.Timezone.ID,
		"utcOffset": payload.Timezone.UTC, "via": via, "updatedAt": time.Now().UnixMilli(),
	})
}
