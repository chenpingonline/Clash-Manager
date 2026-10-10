package mihomo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDelayClientRoutesProviderMembersAndCachesConcurrentDiscovery(t *testing.T) {
	var discoveries atomic.Int32
	const provider = "订阅 / A?#%"
	const node = "香港 / 01?#%"
	const testURL = "http://example.invalid/generate_204?a=1&b=2"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing Controller secret")
		}
		switch r.URL.EscapedPath() {
		case "/proxies/" + url.PathEscape(node) + "/delay":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		case "/providers/proxies":
			discoveries.Add(1)
			_, _ = fmt.Fprintf(w, `{"providers":{%q:{"proxies":[{"name":%q},{"name":%q}]}}}`, provider, node, node)
		case "/providers/proxies/" + url.PathEscape(provider) + "/" + url.PathEscape(node) + "/healthcheck":
			if r.URL.Query().Get("url") != testURL || r.URL.Query().Get("timeout") != "5000" {
				t.Errorf("unexpected query: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"delay":42}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.EscapedPath())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewDelayClient(&Client{SettingsFile: writeSettings(t, Settings{Controller: server.URL, Secret: "test-secret"})})
	var workers sync.WaitGroup
	for i := 0; i < 10; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			response, err := client.Do(context.Background(), node, testURL, 5000)
			if err != nil {
				t.Error(err)
				return
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if string(body) != `{"delay":42}` {
				t.Errorf("unexpected body: %s", body)
			}
		}()
	}
	workers.Wait()
	if discoveries.Load() != 1 {
		t.Fatalf("provider discovery requests = %d", discoveries.Load())
	}
}

func TestDelayClientDoesNotFallbackForSuccessfulOrFailedGlobalProxy(t *testing.T) {
	for _, status := range []int{200, 401, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/proxies/local/delay" {
					t.Errorf("unexpected fallback: %s", r.URL.Path)
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"delay":12,"message":"Request Timeout"}`)
			}))
			defer server.Close()
			client := NewDelayClient(&Client{SettingsFile: writeSettings(t, Settings{Controller: server.URL})})
			response, err := client.Do(context.Background(), "local", "http://example.invalid", 5000)
			if status == 200 {
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
			} else {
				var apiError *APIError
				if !errors.As(err, &apiError) || apiError.Status != status {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDelayClientReportsMissingAmbiguousAndUnavailableProviders(t *testing.T) {
	for _, tc := range []struct {
		label, body                string
		providerStatus, wantStatus int
	}{
		{"missing", `{"providers":{}}`, 200, 404},
		{"ambiguous", `{"providers":{"A":{"proxies":[{"name":"node"}]},"B":{"proxies":[{"name":"node"}]}}}`, 200, 409},
		{"unavailable", `{"message":"Unavailable"}`, 503, 503},
	} {
		t.Run(tc.label, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxies/node/delay" {
					w.WriteHeader(404)
					return
				}
				if r.URL.Path != "/providers/proxies" {
					t.Errorf("unexpected node selected: %s", r.URL.Path)
				}
				w.WriteHeader(tc.providerStatus)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := NewDelayClient(&Client{SettingsFile: writeSettings(t, Settings{Controller: server.URL})})
			_, err := client.Do(context.Background(), "node", "http://example.invalid", 5000)
			var apiError *APIError
			if !errors.As(err, &apiError) || apiError.Status != tc.wantStatus {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
