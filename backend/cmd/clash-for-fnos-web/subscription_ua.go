package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const vergeReleaseURL = "https://api.github.com/repos/clash-verge-rev/clash-verge-rev/releases/latest"

var stableVergeTag = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

func subscriptionOverride(value string) string {
	value = strings.TrimSpace(value)
	if version, ok := strings.CutPrefix(value, "clash-verge/v"); ok && stableVergeTag.MatchString("v"+version) {
		return ""
	}
	return value
}

type vergeReleaseCache struct {
	mu        sync.Mutex
	value     string
	nextCheck time.Time
	url       string
}

func (c *vergeReleaseCache) current() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentLocked()
}
func (c *vergeReleaseCache) currentLocked() string {
	if c.value != "" {
		return c.value
	}
	return defaultSubscriptionUA
}

func (c *vergeReleaseCache) resolve(ctx context.Context) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.url == "" || time.Now().Before(c.nextCheck) {
		return c.currentLocked()
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return c.currentLocked()
	}
	// Retry transient failures sooner than successful lookups.
	c.nextCheck = time.Now().Add(time.Minute)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.currentLocked()
	}
	defer resp.Body.Close()
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release) == nil && !release.Draft && !release.Prerelease && stableVergeTag.MatchString(release.Tag) {
		c.value = "clash-verge/v" + strings.TrimPrefix(release.Tag, "v")
		c.nextCheck = time.Now().Add(5 * time.Minute)
	}
	return c.currentLocked()
}
