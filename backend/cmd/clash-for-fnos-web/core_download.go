package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const coreDownloadMaxSize int64 = 80 << 20

var errCoreDownloadIdle = errors.New("download idle timeout")

type coreDownloadPolicy struct {
	totalTimeout, idleTimeout, retryDelay time.Duration
	attempts                              int
}

var defaultCoreDownloadPolicy = coreDownloadPolicy{10 * time.Minute, 45 * time.Second, time.Second, 3}

type coreDownloadProgress struct {
	downloaded, total    int64
	attempt, maxAttempts int
	retrying             bool
}

func (p coreDownloadProgress) percent() *int {
	if p.total <= 0 {
		return nil
	}
	n := int(min(int64(100), p.downloaded*100/p.total))
	return &n
}

func coreUpdateStageError(stage string, err error) error {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() {
		return fmt.Errorf("%s超时：%w", stage, err)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s已取消：%w", stage, err)
	}
	return fmt.Errorf("%s失败：%w", stage, err)
}

// One total budget covers every attempt. An idle deadline applies while waiting
// for headers or bytes; receiving data renews it. Only downloaded bytes count.
func downloadCore(ctx context.Context, client *http.Client, url string, total int64, policy coreDownloadPolicy, update func(coreDownloadProgress)) ([]byte, error) {
	if total > coreDownloadMaxSize {
		return nil, errors.New("下载失败：Mihomo Core 超过大小限制")
	}
	ctx, cancel := context.WithTimeout(ctx, policy.totalTimeout)
	defer cancel()
	var body bytes.Buffer
	progress := coreDownloadProgress{total: total, maxAttempts: policy.attempts}
	report := func() {
		progress.downloaded = int64(body.Len())
		if update != nil {
			update(progress)
		}
	}
	var lastErr error
	for attempt := 1; attempt <= policy.attempts; attempt++ {
		progress.attempt = attempt
		progress.retrying = false
		report()
		var retry bool
		lastErr, retry = downloadCoreAttempt(ctx, client, url, &body, &progress, policy, report)
		if lastErr == nil {
			report()
			return body.Bytes(), nil
		}
		report()
		if !retry || ctx.Err() != nil || attempt == policy.attempts {
			break
		}
		progress.retrying = true
		report()
		timer := time.NewTimer(policy.retryDelay * time.Duration(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			lastErr = coreUpdateStageError("下载", ctx.Err())
			return body.Bytes(), lastErr
		case <-timer.C:
		}
	}
	return body.Bytes(), fmt.Errorf("%w（已尝试 %d 次）", lastErr, progress.attempt)
}

func downloadCoreAttempt(ctx context.Context, client *http.Client, url string, body *bytes.Buffer, progress *coreDownloadProgress, policy coreDownloadPolicy, report func()) (error, bool) {
	attemptCtx, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(policy.idleTimeout, func() { cancel(errCoreDownloadIdle) })
	defer func() { timer.Stop(); cancel(nil) }()
	failure := func(err error) error {
		if errors.Is(context.Cause(attemptCtx), errCoreDownloadIdle) {
			return fmt.Errorf("下载超时：连续 %g 秒未收到数据", policy.idleTimeout.Seconds())
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return coreUpdateStageError("下载", err)
	}
	offset := int64(body.Len())
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
	if err != nil {
		return failure(err), false
	}
	req.Header.Set("User-Agent", "Clash-for-fnos/v"+version)
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := client.Do(req)
	if err != nil {
		return failure(err), true
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("下载失败：HTTP %d", response.StatusCode), response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500
	}
	if response.StatusCode == http.StatusPartialContent {
		if !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes ") {
			return errors.New("下载失败：续传响应缺少有效 Content-Range"), false
		}
		rangeText := strings.TrimPrefix(response.Header.Get("Content-Range"), "bytes ")
		bounds, sizeText, ok := strings.Cut(rangeText, "/")
		first, last, hasBounds := strings.Cut(bounds, "-")
		start, e1 := strconv.ParseInt(first, 10, 64)
		end, e2 := strconv.ParseInt(last, 10, 64)
		size, e3 := strconv.ParseInt(sizeText, 10, 64)
		if !ok || !hasBounds || e1 != nil || e2 != nil || e3 != nil || start != offset || end < start || end >= size || size > coreDownloadMaxSize || progress.total > 0 && progress.total != size || response.ContentLength >= 0 && response.ContentLength != end-start+1 {
			return errors.New("下载失败：续传范围或文件大小不匹配"), false
		}
		progress.total = size
	} else {
		// A server may ignore Range. Start over rather than append duplicate bytes.
		body.Reset()
		if progress.total <= 0 && response.ContentLength > 0 {
			progress.total = response.ContentLength
		}
		if progress.total > coreDownloadMaxSize || response.ContentLength > coreDownloadMaxSize {
			return errors.New("下载失败：Mihomo Core 超过大小限制"), false
		}
		report()
	}
	timer.Reset(policy.idleTimeout)
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			timer.Reset(policy.idleTimeout)
			if int64(body.Len()+n) > coreDownloadMaxSize || progress.total > 0 && int64(body.Len()+n) > progress.total {
				return errors.New("下载失败：Mihomo Core 超过声明的文件大小"), false
			}
			_, _ = body.Write(buffer[:n])
			report()
		}
		if readErr != nil {
			if readErr != io.EOF {
				return failure(readErr), true
			}
			if attemptCtx.Err() != nil {
				return failure(attemptCtx.Err()), true
			}
			if progress.total > 0 && int64(body.Len()) != progress.total {
				return failure(io.ErrUnexpectedEOF), true
			}
			if body.Len() == 0 {
				return errors.New("下载失败：服务器返回空文件"), true
			}
			return nil, false
		}
	}
}
