package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"
)

const maxExitLocationResponse = 128 << 10
const exitLocationTimeout = 5 * time.Second
const exitLocationBudget = 33 * time.Second

// All services describe the source address of this request. Do not infer that
// the result belongs to a selected proxy group: system routing/TUN decides it.
var defaultExitLocationURLs = []string{
	"https://ipwho.is/?lang=zh-CN&fields=success,message,ip,country,country_code,region,city,timezone",
	"https://api.ip.sb/geoip",
	"https://get.geojs.io/v1/ip/geo.json",
	"https://ipapi.co/json",
	"https://api.ipapi.is/",
	"https://ip.api.skk.moe/cf-geoip",
}

type exitLocationPayload struct {
	Error       bool   `json:"error"`
	CountryName string `json:"country_name"`
	UTCOffset   string `json:"utc_offset"`
	Location    *struct {
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		State       string `json:"state"`
		City        string `json:"city"`
		Timezone    string `json:"timezone"`
	} `json:"location"`
	Success     *bool           `json:"success"`
	IP          string          `json:"ip"`
	Country     string          `json:"country"`
	CountryCode string          `json:"country_code"`
	Region      string          `json:"region"`
	City        string          `json:"city"`
	Timezone    json.RawMessage `json:"timezone"`
}

func fetchExitLocation(ctx context.Context, client *http.Client, endpoint string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("检测服务地址无效")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Clash-for-fnOS/"+version)
	response, err := client.Do(request)
	// Transport errors can contain proxy credentials or an override URL's token.
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("请求超时或已取消")
		}
		return nil, fmt.Errorf("网络连接失败，请检查系统代理、DNS 与 TUN 分流")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxExitLocationResponse+1))
	if err != nil || len(body) > maxExitLocationResponse {
		return nil, fmt.Errorf("响应读取失败或过大")
	}
	var payload exitLocationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("检测服务返回了无效数据")
	}
	payload.IP = strings.TrimSpace(payload.IP)
	if payload.Error || (payload.Success != nil && !*payload.Success) || net.ParseIP(payload.IP) == nil {
		return nil, fmt.Errorf("检测服务未返回有效出口 IP")
	}
	// ipapi.co uses country for the ISO code and country_name for its name.
	if payload.CountryName != "" {
		if payload.CountryCode == "" {
			payload.CountryCode = payload.Country
		}
		payload.Country = payload.CountryName
	}
	// skk.moe returns only the country code in country.
	if payload.CountryCode == "" && len(payload.Country) == 2 {
		payload.CountryCode = payload.Country
	}
	var timezone string
	utc := payload.UTCOffset
	if len(payload.Timezone) > 0 && string(payload.Timezone) != "null" {
		if err := json.Unmarshal(payload.Timezone, &timezone); err != nil {
			var zone struct {
				ID  string `json:"id"`
				UTC string `json:"utc"`
			}
			if err := json.Unmarshal(payload.Timezone, &zone); err != nil {
				return nil, fmt.Errorf("检测服务返回了无效时区")
			}
			timezone, utc = zone.ID, zone.UTC
		}
	}
	// ipapi.is nests its geographic fields under location.
	if payload.Location != nil {
		payload.Country = payload.Location.Country
		payload.CountryCode = payload.Location.CountryCode
		payload.Region = payload.Location.State
		payload.City = payload.Location.City
		timezone = payload.Location.Timezone
	}
	return map[string]any{
		"ip": payload.IP, "country": payload.Country, "countryCode": strings.ToUpper(payload.CountryCode),
		"region": payload.Region, "city": payload.City, "timezone": timezone,
		"utcOffset": utc, "via": "system", "updatedAt": time.Now().UnixMilli(),
	}, nil
}

func (g *gateway) writeExitLocation(w http.ResponseWriter, r *http.Request) {
	// Use the host network and standard proxy environment, just like a native
	// HTTP client. Never synthesize a loopback proxy from Mihomo's input ports.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: exitLocationTimeout}
	endpoints := append([]string(nil), defaultExitLocationURLs...)
	rand.Shuffle(len(endpoints), func(i, j int) { endpoints[i], endpoints[j] = endpoints[j], endpoints[i] })
	// Preserve the existing explicit override as a single-service configuration.
	if endpoint := strings.TrimSpace(g.config.exitLocationURL); endpoint != "" {
		endpoints = []string{endpoint}
	}
	ctx, cancel := context.WithTimeout(r.Context(), exitLocationBudget)
	defer cancel()
	result, err := queryExitLocations(ctx, client, endpoints)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出口位置更新失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func queryExitLocations(ctx context.Context, client *http.Client, endpoints []string) (map[string]any, error) {
	failures := make([]string, 0, len(endpoints))
	for i, endpoint := range endpoints {
		if ctx.Err() != nil {
			break
		}
		attemptCtx, stop := context.WithTimeout(ctx, exitLocationTimeout)
		result, err := fetchExitLocation(attemptCtx, client, endpoint)
		stop()
		if err == nil {
			return result, nil
		}
		failures = append(failures, fmt.Sprintf("服务 %d：%v", i+1, err))
	}
	if ctx.Err() != nil {
		failures = append(failures, "查询已超时或取消")
	}
	return nil, fmt.Errorf("%s", strings.Join(failures, "；"))
}
