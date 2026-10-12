package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestDelayProgressKeepsSubmittedGroupsSeparateFromSharedTargets(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single group"
		if batch {
			name = "group batch"
		}
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			var calls atomic.Int32
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/proxies":
					_, _ = io.WriteString(w, `{"proxies":{"AI":{"all":["shared"]},"Video":{"all":["shared"]},"Other":{"all":["shared"]}}}`)
				case "/proxies/shared/delay":
					calls.Add(1)
					<-release
					_, _ = io.WriteString(w, `{"delay":23}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer controller.Close()
			// Always release workers before server cleanup, including failed assertions.
			released := false
			defer func() {
				if !released {
					close(release)
				}
			}()
			g := newGateway(config{settingsFile: writeGatewaySettings(t, controller.URL)})
			proxies := httptest.NewRecorder()
			g.ServeHTTP(proxies, httptest.NewRequest("GET", "/api/proxies", nil))
			var current struct {
				Context groupDelayContext `json:"delayContext"`
			}
			if proxies.Code != 200 || json.Unmarshal(proxies.Body.Bytes(), &current) != nil {
				t.Fatalf("load: %d %s", proxies.Code, proxies.Body.String())
			}
			request := delayBatchRequest{Scope: current.Context.Scope, Group: "AI", Names: []string{"shared"}}
			wantGroups := []string{"AI"}
			if batch {
				request.Group, request.Names = "", nil
				request.Groups = []delayGroupRequest{{Group: "AI", Names: []string{"shared"}}, {Group: "Video", Names: []string{"shared"}}}
				wantGroups = append(wantGroups, "Video")
			}
			raw, _ := json.Marshal(request)
			start := httptest.NewRecorder()
			g.ServeHTTP(start, httptest.NewRequest("POST", "/api/delays", bytes.NewReader(raw)))
			if start.Code != 202 {
				t.Fatalf("start: %d %s", start.Code, start.Body.String())
			}
			readStatus := func() delayBatchStatus {
				t.Helper()
				response := httptest.NewRecorder()
				g.ServeHTTP(response, httptest.NewRequest("GET", "/api/delays", nil))
				var status delayBatchStatus
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &status) != nil {
					t.Fatalf("status: %d %s", response.Code, response.Body.String())
				}
				if !reflect.DeepEqual(status.Groups, wantGroups) || status.GroupBatch != batch || len(status.Targets) != 1 {
					t.Fatalf("incorrect group progress after target de-duplication: %#v", status)
				}
				return status
			}
			if readStatus().State != "running" {
				t.Fatal("job should be running before release")
			}
			close(release)
			released = true
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				status := readStatus()
				if status.State == "done" {
					if calls.Load() != 1 || len(status.Results) != 1 || status.Results[0].Delay != 23 {
						t.Fatalf("shared target was measured more than once: %#v", status)
					}
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatal("job did not complete")
		})
	}
}
