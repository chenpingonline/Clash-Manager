package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func downloadTestPolicy() coreDownloadPolicy {
	return coreDownloadPolicy{time.Second, 200 * time.Millisecond, time.Millisecond, 3}
}

func TestCoreDownloadResumesAfterTruncation(t *testing.T) {
	for _, ignoreRange := range []bool{false, true} {
		t.Run(fmt.Sprint(ignoreRange), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					w.Header().Set("Content-Length", "10")
					_, _ = w.Write([]byte("abcd"))
					return
				}
				if r.Header.Get("Range") != "bytes=4-" {
					t.Errorf("range=%s", r.Header.Get("Range"))
				}
				if ignoreRange {
					_, _ = w.Write([]byte("abcdefghij"))
					return
				}
				w.Header().Set("Content-Range", "bytes 4-9/10")
				w.Header().Set("Content-Length", "6")
				w.WriteHeader(206)
				_, _ = w.Write([]byte("efghij"))
			}))
			defer server.Close()
			sawPartial, sawRetry := false, false
			body, err := downloadCore(context.Background(), server.Client(), server.URL, 10, downloadTestPolicy(), func(p coreDownloadProgress) {
				if p.downloaded == 4 && p.percent() != nil && *p.percent() == 40 {
					sawPartial = true
				}
				if p.retrying {
					sawRetry = true
				}
			})
			if err != nil || string(body) != "abcdefghij" || requests.Load() != 2 || !sawPartial || !sawRetry {
				t.Fatalf("body=%s requests=%d partial=%v retry=%v err=%v", body, requests.Load(), sawPartial, sawRetry, err)
			}
		})
	}
}

func TestCoreDownloadIdleTimeoutPreservesBytes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte("abcd"))
		} else {
			w.Header().Set("Content-Length", "6")
			w.Header().Set("Content-Range", "bytes 4-9/10")
			w.WriteHeader(206)
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	policy := downloadTestPolicy()
	policy.idleTimeout = 40 * time.Millisecond
	var last coreDownloadProgress
	body, err := downloadCore(context.Background(), server.Client(), server.URL, 10, policy, func(p coreDownloadProgress) { last = p })
	if err == nil || !strings.Contains(err.Error(), "下载超时") || !strings.Contains(err.Error(), "已尝试 3 次") || string(body) != "abcd" || last.downloaded != 4 || last.percent() == nil || *last.percent() != 40 || requests.Load() != 3 {
		t.Fatalf("body=%s last=%+v requests=%d err=%v", body, last, requests.Load(), err)
	}
}

func TestCoreDownloadReceivingDataRenewsIdleDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "6")
		for _, b := range []byte("abcdef") {
			_, _ = w.Write([]byte{b})
			w.(http.Flusher).Flush()
			time.Sleep(25 * time.Millisecond)
		}
	}))
	defer server.Close()
	policy := downloadTestPolicy()
	policy.idleTimeout = 100 * time.Millisecond
	body, err := downloadCore(context.Background(), server.Client(), server.URL, 6, policy, nil)
	if err != nil || string(body) != "abcdef" {
		t.Fatalf("%s: %v", body, err)
	}
}

func TestCoreDownloadHTTPRetryClassification(t *testing.T) {
	for _, status := range []int{503, 429, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					w.WriteHeader(status)
					return
				}
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()
			body, err := downloadCore(context.Background(), server.Client(), server.URL, 2, downloadTestPolicy(), nil)
			if status == 404 {
				if err == nil || requests.Load() != 1 {
					t.Fatalf("retried permanent error: %v", err)
				}
				return
			}
			if err != nil || string(body) != "ok" || requests.Load() != 2 {
				t.Fatalf("%s %v", body, err)
			}
		})
	}
}

func TestCoreDownloadRejectsWrongResumeRange(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte("abcd"))
			return
		}
		w.Header().Set("Content-Range", "bytes 0-5/10")
		w.WriteHeader(206)
		_, _ = w.Write([]byte("efghij"))
	}))
	defer server.Close()
	body, err := downloadCore(context.Background(), server.Client(), server.URL, 10, downloadTestPolicy(), nil)
	if err == nil || !strings.Contains(err.Error(), "续传范围") || string(body) != "abcd" || requests.Load() != 2 {
		t.Fatalf("%s %v", body, err)
	}
}

func TestCoreDownloadTotalDeadlineStopsRetries(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	policy := downloadTestPolicy()
	policy.totalTimeout = 50 * time.Millisecond
	policy.retryDelay = time.Second
	_, err := downloadCore(context.Background(), server.Client(), server.URL, 10, policy, nil)
	if err == nil || !strings.Contains(err.Error(), "下载超时") || requests.Load() != 1 {
		t.Fatalf("requests=%d err=%v", requests.Load(), err)
	}
}

func TestCoreDownloadCancellationAndSizeLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := downloadCore(ctx, &http.Client{}, "http://127.0.0.1:1", 10, downloadTestPolicy(), nil)
	if err == nil || !strings.Contains(err.Error(), "下载已取消") {
		t.Fatal(err)
	}
	_, err = downloadCore(context.Background(), &http.Client{}, "http://127.0.0.1:1", coreDownloadMaxSize+1, downloadTestPolicy(), nil)
	if err == nil || !strings.Contains(err.Error(), "大小限制") {
		t.Fatal(err)
	}
}

func TestCoreUpdateTimeoutNamesPhase(t *testing.T) {
	for _, phase := range []string{"下载", "安装内核", "等待内核重启"} {
		err := coreUpdateStageError(phase, context.DeadlineExceeded)
		if !strings.HasPrefix(err.Error(), phase+"超时") {
			t.Fatal(err)
		}
	}
}
