package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/mihomo"
)

const maxDelayBatchConcurrency = 10

type delayGroupRequest struct {
	Group string   `json:"group"`
	Names []string `json:"names"`
}
type delayBatchRequest struct {
	Names  []string            `json:"names"`
	Group  string              `json:"group,omitempty"`
	Scope  string              `json:"scope,omitempty"`
	Groups []delayGroupRequest `json:"groups,omitempty"`
}
type delayBatchTarget struct {
	Name     string `json:"name"`
	Scope    string `json:"scope,omitempty"`
	Group    string `json:"group,omitempty"`
	URL      string `json:"url,omitempty"`
	Expected string `json:"expected,omitempty"`
}

func delayTargetKey(target delayBatchTarget) string {
	raw, _ := json.Marshal([]string{target.Scope, target.URL, target.Expected, target.Name})
	return string(raw)
}

type delayBatchResult struct {
	delayBatchTarget
	Delay int    `json:"delay,omitempty"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type delayBatchJob struct {
	ID        string             `json:"jobId,omitempty"`
	Targets   []delayBatchTarget `json:"targets,omitempty"`
	State     string             `json:"state"`
	Names     []string           `json:"names"`
	CreatedAt int64              `json:"createdAt,omitempty"`
	UpdatedAt int64              `json:"updatedAt,omitempty"`
}

type delayBatchStatus struct {
	delayBatchJob
	Results []delayBatchResult `json:"results"`
}

func normalizedDelayNames(names []string) ([]string, error) {
	if len(names) > 2000 {
		return nil, errors.New("单次测速节点不能超过 2000 个")
	}
	seen := make(map[string]struct{}, len(names))
	result := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || len(name) > 512 || strings.ContainsRune(name, '\x00') {
			return nil, errors.New("测速节点名称无效")
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil, errors.New("没有可测速节点")
	}
	return result, nil
}

func delayResult(ctx context.Context, client *mihomo.DelayClient, name, testURL string, timeout int, expected ...string) delayBatchResult {
	response, err := client.Do(ctx, name, testURL, timeout, expected...)
	if err != nil {
		state := "error"
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() || strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(err.Error(), "超时") {
			state = "timeout"
		}
		return delayBatchResult{delayBatchTarget: delayBatchTarget{Name: name}, State: state, Error: err.Error()}
	}
	defer response.Body.Close()
	var payload struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return delayBatchResult{delayBatchTarget: delayBatchTarget{Name: name}, State: "error", Error: "解析测速结果失败: " + err.Error()}
	}
	if payload.Delay <= 0 {
		return delayBatchResult{delayBatchTarget: delayBatchTarget{Name: name}, State: "error", Error: "测速未返回有效延迟"}
	}
	return delayBatchResult{delayBatchTarget: delayBatchTarget{Name: name}, Delay: payload.Delay, State: "done"}
}

func (g *gateway) delayStatusLocked() delayBatchStatus {
	status := delayBatchStatus{delayBatchJob: delayBatchJob{State: "idle", Names: []string{}}, Results: make([]delayBatchResult, 0, len(g.delayResults))}
	if g.delayJob != nil {
		status.delayBatchJob = *g.delayJob
		status.Names = append([]string(nil), g.delayJob.Names...)
		status.Targets = append([]delayBatchTarget(nil), g.delayJob.Targets...)
	}
	keys := make([]string, 0, len(g.delayResults))
	for name := range g.delayResults {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		status.Results = append(status.Results, g.delayResults[name])
	}
	return status
}

func (g *gateway) publishDelayStatusLocked() {
	status := g.delayStatusLocked()
	for updates := range g.delayWatchers {
		select {
		case updates <- status:
		default:
			select {
			case <-updates:
			default:
			}
			select {
			case updates <- status:
			default:
			}
		}
	}
}

func (g *gateway) startDelayBatch(w http.ResponseWriter, r *http.Request, client *mihomo.Client) {
	body, ok := readLimitedBody(w, r, 1<<20)
	if !ok {
		return
	}
	var request delayBatchRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "测速请求格式错误"})
		return
	}

	// Freeze settings and source identity before accepting the job. A stale page
	// must not save/test against another subscription's identically named group.
	g.configMu.Lock()
	defer g.configMu.Unlock()
	client, err := client.Snapshot()
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	settings, _ := client.LoadSettings()
	scope, err := g.groupDelayScope(client)
	if err != nil {
		writeMihomoError(w, err)
		return
	}
	if request.Scope != "" && request.Scope != scope {
		writeJSON(w, 409, map[string]string{"error": "当前配置已切换，请刷新后重试"})
		return
	}
	var targets []delayBatchTarget
	if request.Group != "" || len(request.Groups) > 0 {
		if request.Scope == "" {
			writeJSON(w, 400, map[string]string{"error": "缺少当前配置标识"})
			return
		}
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
		groups := request.Groups
		if request.Group != "" {
			groups = []delayGroupRequest{{Group: request.Group, Names: request.Names}}
		}
		proxies, _ := payload["proxies"].(map[string]any)
		seen := map[string]bool{}
		for _, group := range groups {
			target, exists := context.Groups[group.Group]
			if !exists {
				writeJSON(w, 400, map[string]string{"error": "代理组不存在"})
				return
			}
			names, err := normalizedDelayNames(group.Names)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			proxy, _ := proxies[group.Group].(map[string]any)
			members, _ := proxy["all"].([]any)
			allowed := map[string]bool{}
			for _, member := range members {
				if name, ok := member.(string); ok {
					allowed[name] = true
				}
			}
			for _, name := range names {
				if !allowed[name] {
					writeJSON(w, 400, map[string]string{"error": "测速节点不属于指定代理组"})
					return
				}
				item := delayBatchTarget{Name: name, Scope: scope, Group: group.Group, URL: target.URL, Expected: target.Expected}
				key := delayTargetKey(item)
				if !seen[key] {
					targets = append(targets, item)
					seen[key] = true
				}
			}
		}
	} else {
		names, err := normalizedDelayNames(request.Names)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		for _, name := range names {
			targets = append(targets, delayBatchTarget{Name: name, Scope: scope, URL: settings.HealthcheckURL})
		}
	}
	if len(targets) == 0 || len(targets) > 2000 {
		writeJSON(w, 400, map[string]string{"error": "单次测速需包含 1～2000 个测试目标"})
		return
	}
	names := []string{}
	for _, target := range targets {
		names = append(names, target.Name)
	}

	g.delayMu.Lock()
	if g.delayJob != nil && g.delayJob.State == "running" {
		status := g.delayStatusLocked()
		g.delayMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "已有测速任务正在进行", "job": status})
		return
	}
	now := time.Now().UnixMilli()
	g.delayJob = &delayBatchJob{ID: newHexID(8), State: "running", Names: names, Targets: targets, CreatedAt: now, UpdatedAt: now}
	status := g.delayStatusLocked()
	g.publishDelayStatusLocked()
	g.delayMu.Unlock()

	go g.runDelayTargets(client, settings.HealthcheckTimeout, status.ID, targets, g.delayGeneration.Load())
	writeJSON(w, http.StatusAccepted, status)
}

func (g *gateway) runDelayTargets(client *mihomo.Client, timeout int, jobID string, targets []delayBatchTarget, generation uint64) {
	delayClient := mihomo.NewDelayClient(client)
	results := make(chan delayBatchResult, maxDelayBatchConcurrency)
	jobs := make(chan delayBatchTarget)
	var workers sync.WaitGroup
	workers.Add(min(maxDelayBatchConcurrency, len(targets)))
	for range min(maxDelayBatchConcurrency, len(targets)) {
		go func() {
			defer workers.Done()
			for target := range jobs {
				if len(targets) > 1 {
					time.Sleep(time.Duration(rand.IntN(201)) * time.Millisecond)
				}
				result := delayResult(context.Background(), delayClient, target.Name, target.URL, timeout, target.Expected)
				result.delayBatchTarget = target
				results <- result
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, target := range targets {
			jobs <- target
		}
	}()
	go func() { workers.Wait(); close(results) }()
	completed := make([]delayBatchResult, 0, len(targets))
	for result := range results {
		completed = append(completed, result)
	}
	g.configMu.Lock()
	defer g.configMu.Unlock()
	scope, scopeErr := g.groupDelayScope(client)
	if g.delayGeneration.Load() != generation || scopeErr != nil || len(targets) > 0 && scope != targets[0].Scope {
		for index := range completed {
			completed[index].Delay = 0
			completed[index].State = "error"
			completed[index].Error = "测速期间配置已切换，请重新测试"
		}
	}
	g.delayMu.Lock()
	defer g.delayMu.Unlock()
	if g.delayJob == nil || g.delayJob.ID != jobID {
		return
	}
	for _, result := range completed {
		g.delayResults[delayTargetKey(result.delayBatchTarget)] = result
	}
	// Results are transient; bound memory while preserving recent target contexts.
	if len(g.delayResults) > 10000 {
		g.delayResults = map[string]delayBatchResult{}
		for _, result := range completed {
			g.delayResults[delayTargetKey(result.delayBatchTarget)] = result
		}
	}
	g.delayJob.State = "done"
	g.delayJob.UpdatedAt = time.Now().UnixMilli()
	g.publishDelayStatusLocked()
}

func (g *gateway) writeDelayStatus(w http.ResponseWriter) {
	g.delayMu.RLock()
	status := g.delayStatusLocked()
	g.delayMu.RUnlock()
	writeJSON(w, http.StatusOK, status)
}

func (g *gateway) streamDelayStatus(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "测速任务不存在或已过期"})
		return
	}
	g.delayMu.Lock()
	if g.delayJob == nil || g.delayJob.ID != id {
		g.delayMu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "测速任务不存在或已过期"})
		return
	}
	initial := g.delayStatusLocked()
	updates := make(chan delayBatchStatus, 1)
	if initial.State == "running" {
		g.delayWatchers[updates] = struct{}{}
	}
	g.delayMu.Unlock()
	if initial.State == "running" {
		defer func() {
			g.delayMu.Lock()
			delete(g.delayWatchers, updates)
			g.delayMu.Unlock()
		}()
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "当前服务不支持流式响应"})
		return
	}
	writeEvent := func(value delayBatchStatus) bool {
		body, err := json.Marshal(value)
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !writeEvent(initial) || initial.State != "running" {
		return
	}
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case next := <-updates:
			if !writeEvent(next) || next.State != "running" {
				return
			}
		case <-keepAlive.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
