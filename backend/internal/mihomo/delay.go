package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DelayClient keeps one provider snapshot for a delay batch. Provider members
// are not in Mihomo's global /proxies table, even when a group lists them.
type DelayClient struct {
	client      *Client
	once        sync.Once
	providers   map[string][]string
	providerErr error
}

func NewDelayClient(client *Client) *DelayClient { return &DelayClient{client: client} }

func (c *DelayClient) loadProviders(ctx context.Context) {
	response, err := c.client.Do(ctx, http.MethodGet, "/providers/proxies", nil, 12*time.Second)
	if err != nil {
		c.providerErr = err
		return
	}
	defer response.Body.Close()
	var payload struct {
		Providers map[string]struct {
			Proxies []struct {
				Name string `json:"name"`
			} `json:"proxies"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 12<<20)).Decode(&payload); err != nil {
		c.providerErr = fmt.Errorf("解析 Mihomo 代理集合失败: %w", err)
		return
	}
	c.providers = make(map[string][]string)
	for provider, entry := range payload.Providers {
		seen := make(map[string]bool)
		for _, proxy := range entry.Proxies {
			if proxy.Name != "" && !seen[proxy.Name] {
				c.providers[proxy.Name] = append(c.providers[proxy.Name], provider)
				seen[proxy.Name] = true
			}
		}
	}
}

func (c *DelayClient) Do(ctx context.Context, name, testURL string, timeout int) (*http.Response, error) {
	query := url.Values{"url": {testURL}, "timeout": {strconv.Itoa(timeout)}}
	requestTimeout := time.Duration(timeout+3000) * time.Millisecond
	response, err := c.client.Do(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?"+query.Encode(), nil, requestTimeout)
	var apiError *APIError
	// Real timeouts, authentication errors and failed proxy connections must not
	// be retried against a different node with the same name.
	if !errors.As(err, &apiError) || apiError.Status != http.StatusNotFound {
		return response, err
	}
	c.once.Do(func() { c.loadProviders(ctx) })
	if c.providerErr != nil {
		return nil, c.providerErr
	}
	providers := c.providers[name]
	if len(providers) == 0 {
		return nil, err
	}
	if len(providers) > 1 {
		ordered := append([]string(nil), providers...)
		sort.Strings(ordered)
		return nil, &APIError{Status: http.StatusConflict, Message: fmt.Sprintf("节点 %s 存在于多个代理集合（%s），请为节点设置唯一名称后测速", name, strings.Join(ordered, "、"))}
	}
	path := "/providers/proxies/" + url.PathEscape(providers[0]) + "/" + url.PathEscape(name) + "/healthcheck?" + query.Encode()
	return c.client.Do(ctx, http.MethodGet, path, nil, requestTimeout)
}
