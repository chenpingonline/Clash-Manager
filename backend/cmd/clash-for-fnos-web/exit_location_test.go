package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExitLocationReturnsNormalizedLiveData(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/configs" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"mixed-port":0,"port":0,"socks-port":0}`))
	}))
	defer controller.Close()
	location := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"ip":"103.123.45.67","country":"中国香港","country_code":"hk","region":"中西区","city":"香港","timezone":{"id":"Asia/Hong_Kong","utc":"+08:00"}}`))
	}))
	defer location.Close()

	handler := newGateway(config{
		gateway:           "/app/clash-for-fnos",
		settingsFile:      writeGatewaySettings(t, controller.URL),
		trafficTotalsFile: filepath.Join(t.TempDir(), "traffic-totals.json"),
		exitLocationURL:   location.URL,
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/exit-location", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{`"ip":"103.123.45.67"`, `"country":"中国香港"`, `"countryCode":"HK"`, `"timezone":"Asia/Hong_Kong"`, `"via":"system"`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("missing %s in %s", expected, recorder.Body.String())
		}
	}
}

func TestExitLocationFallbackAndFormats(t *testing.T) {
	for _, data := range []string{
		`{"ip":"203.0.113.1","country_code":"hk","timezone":"Asia/Hong_Kong"}`,
		`{"success":true,"ip":"2001:db8::1","country_code":"hk","timezone":{"id":"Asia/Hong_Kong","utc":"+08:00"}}`,
	} {
		t.Run(data, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/limited":
					w.WriteHeader(429)
				case "/html":
					_, _ = w.Write([]byte(`<html>blocked</html>`))
				case "/invalid":
					_, _ = w.Write([]byte(`{"ip":"not-an-ip"}`))
				case "/rejected":
					_, _ = w.Write([]byte(`{"success":false,"ip":"203.0.113.2"}`))
				case "/ok":
					_, _ = w.Write([]byte(data))
				default:
					t.Error("continued after success")
				}
			}))
			defer server.Close()
			var endpoints []string
			for _, path := range []string{"/limited", "/html", "/invalid", "/rejected", "/ok", "/unused"} {
				endpoints = append(endpoints, server.URL+path)
			}
			result, err := queryExitLocations(context.Background(), server.Client(), endpoints)
			if err != nil || result["timezone"] != "Asia/Hong_Kong" || result["countryCode"] != "HK" || len(paths) != 5 {
				t.Fatalf("result=%v err=%v paths=%v", result, err, paths)
			}
		})
	}
}

func TestExitLocationTimeoutFallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte(`{"ip":"203.0.113.8"}`))
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 30 * time.Millisecond
	result, err := queryExitLocations(context.Background(), client, []string{server.URL + "/slow", server.URL + "/ok"})
	if err != nil || result["ip"] != "203.0.113.8" {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestExitLocationCancellationStopsFallback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := queryExitLocations(ctx, server.Client(), []string{server.URL, server.URL})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestExitLocationDoesNotReadControllerOrHelper(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("queried controller for proxy ports")
		w.WriteHeader(500)
	}))
	defer controller.Close()
	location := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("forwarded local proxy credentials")
		}
		_, _ = w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer location.Close()
	g := newGateway(config{gateway: "/app/clash-for-fnos", settingsFile: writeGatewaySettings(t, controller.URL), privilegedSocket: "/nonexistent/helper.sock", exitLocationURL: location.URL})
	recorder := httptest.NewRecorder()
	g.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/app/clash-for-fnos/api/exit-location", nil))
	if recorder.Code != 200 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestExitLocationFailureDoesNotLeakURLCredentials(t *testing.T) {
	_, err := queryExitLocations(context.Background(), &http.Client{Timeout: time.Second}, []string{"http://secret-user:secret-password@127.0.0.1:1/?token=secret-token"})
	if err == nil || strings.Contains(err.Error(), "secret-") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestExitLocationSixProviderMappings(t *testing.T) {
	tests := []struct{ name, body, country, code, region, city, zone, utc string }{
		{"ipwho.is", `{"success":true,"ip":"203.0.113.1","country":"Hong Kong","country_code":"HK","region":"Central","city":"Hong Kong","timezone":{"id":"Asia/Hong_Kong","utc":"+08:00"}}`, "Hong Kong", "HK", "Central", "Hong Kong", "Asia/Hong_Kong", "+08:00"},
		{"ip.sb", `{"ip":"203.0.113.1","country":"Hong Kong","country_code":"HK","region":"Central","city":"Hong Kong","timezone":"Asia/Hong_Kong"}`, "Hong Kong", "HK", "Central", "Hong Kong", "Asia/Hong_Kong", ""},
		{"GeoJS", `{"ip":"203.0.113.1","country":"Hong Kong","country_code":"HK","region":"Central","city":"Hong Kong","timezone":"Asia/Hong_Kong"}`, "Hong Kong", "HK", "Central", "Hong Kong", "Asia/Hong_Kong", ""},
		{"ipapi.co", `{"ip":"203.0.113.1","country":"HK","country_name":"Hong Kong","region":"Central","city":"Hong Kong","timezone":"Asia/Hong_Kong","utc_offset":"+0800"}`, "Hong Kong", "HK", "Central", "Hong Kong", "Asia/Hong_Kong", "+0800"},
		{"ipapi.is", `{"ip":"203.0.113.1","location":{"country":"Hong Kong","country_code":"HK","state":"Central","city":"Hong Kong","timezone":"Asia/Hong_Kong"}}`, "Hong Kong", "HK", "Central", "Hong Kong", "Asia/Hong_Kong", ""},
		{"skk.moe", `{"ip":"203.0.113.1","country":"HK","region":"Central","city":"Hong Kong","timezone":"Asia/Hong_Kong"}`, "HK", "HK", "Central", "Hong Kong", "Asia/Hong_Kong", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(test.body)) }))
			defer server.Close()
			result, err := fetchExitLocation(context.Background(), server.Client(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"country": test.country, "countryCode": test.code, "region": test.region, "city": test.city, "timezone": test.zone, "utcOffset": test.utc} {
				if result[key] != want {
					t.Errorf("%s=%v want %s", key, result[key], want)
				}
			}
		})
	}
	if len(defaultExitLocationURLs) != len(tests) {
		t.Fatalf("provider count=%d", len(defaultExitLocationURLs))
	}
	// Each provider gets a full attempt even when all preceding providers time out.
	if exitLocationBudget < time.Duration(len(defaultExitLocationURLs))*exitLocationTimeout {
		t.Fatal("budget cannot reach all providers")
	}
}
