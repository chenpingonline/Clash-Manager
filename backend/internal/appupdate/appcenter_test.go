package appupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fakeCenter(t *testing.T, handler http.Handler) *appCenter {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "up-rpc-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "daemon.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	c := newAppCenter(socket)
	c.pollInterval = time.Millisecond
	t.Cleanup(func() { c.client.CloseIdleConnections(); server.Close(); os.RemoveAll(dir) })
	return c
}

func centerReply(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "test daemon", "data": data})
}

func readyStage() map[string]any {
	return map[string]any{"status": 2, "appName": AppName, "version": "1.3.16", "packageType": "path", "installed": true}
}

func TestCenterUpgradeUsesDaemonAndExistingVolume(t *testing.T) {
	filename, digest := fixture(t, "", "")
	var mu sync.Mutex
	var routes []string
	var statusPolls atomic.Int32
	c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		routes = append(routes, r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/rpc/v1/download/task":
			if body["path"] != filename || body["packageSourceType"] != "file" {
				t.Errorf("staging payload: %#v", body)
			}
			centerReply(w, 0, map[string]any{"downloadTaskId": "download-one"})
		case "/rpc/v1/download/status":
			if body["downloadTaskId"] != "download-one" {
				t.Errorf("staging ID: %#v", body)
			}
			// fnOS reports a file source but a path package. Replay the initial
			// running response and the finished shape observed on the user's NAS.
			if statusPolls.Add(1) == 1 {
				centerReply(w, 0, map[string]any{"status": 1, "message": "running", "progress": 0, "packageType": "path", "packageSourceType": "file", "path": "", "appName": "", "version": "", "installed": false})
			} else {
				stage := readyStage()
				stage["packageSourceType"] = "file"
				stage["path"] = "/vol3/appcenter-downloads/clash-for-fnos-1.3.16-tpk"
				stage["installedInfo"] = map[string]any{"version": "1.3.15", "status": "running", "canUpgrade": true}
				centerReply(w, 0, stage)
			}
		case "/rpc/v1/update/info", "/rpc/v1/update/task":
			if body["appName"] != AppName || body["updateVersion"] != "1.3.16" || body["packageType"] != "path" {
				t.Errorf("update identity: %#v", body)
			}
			if r.URL.Path == "/rpc/v1/update/info" {
				centerReply(w, 0, map[string]any{"wizardInfo": map[string]any{"installedVolumeID": 3}})
			} else {
				params, _ := body["systemParameters"].(map[string]any)
				if params["installVolumeID"] != float64(3) || params["dataVolumeId"] != float64(3) || params["immediateStart"] != true || params["agreedToProtocol"] != true {
					t.Errorf("existing volume/start parameters: %#v", params)
				}
				if custom, ok := body["customParameters"].([]any); !ok || len(custom) != 0 {
					t.Errorf("custom parameters: %#v", body)
				}
				centerReply(w, 0, map[string]any{"taskId": "upgrade-one"})
			}
		case "/rpc/v1/common/status":
			if body["taskId"] != "upgrade-one" {
				t.Errorf("task ID: %#v", body)
			}
			centerReply(w, 0, map[string]any{"status": 2})
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	var log bytes.Buffer
	c.log = &log
	c.installedVersion = func() (string, error) { return "1.3.16", nil }
	var recorded string
	err := installVerified(context.Background(), filename, digest, "1.3.16", "amd64", func(string, int) error { return nil }, func() error {
		return c.upgrade(context.Background(), filename, "1.3.16", func(id string) error { recorded = id; return nil })
	}, func() error { t.Error("unnecessary start fallback"); return nil }, func(time.Duration) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if recorded != "upgrade-one" {
		t.Fatalf("recorded ID = %q", recorded)
	}
	if !strings.Contains(log.String(), `downloadTaskId="download-one"`) || !strings.Contains(log.String(), `"packageType":"path"`) {
		t.Fatalf("missing staging diagnostics: %s", log.String())
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"/rpc/v1/download/task", "/rpc/v1/download/status", "/rpc/v1/download/status", "/rpc/v1/update/info", "/rpc/v1/update/task", "/rpc/v1/common/status"}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %v", routes)
	}
}

func TestCenterRejectsStagingMismatchBeforeUpgrade(t *testing.T) {
	for _, field := range []string{"appName", "version", "packageType", "installed"} {
		t.Run(field, func(t *testing.T) {
			c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rpc/v1/download/task":
					centerReply(w, 0, map[string]any{"downloadTaskId": "one"})
				case "/rpc/v1/download/status":
					stage := readyStage()
					stage[field] = "wrong"
					if field == "packageType" {
						stage[field] = ""
					}
					if field == "installed" {
						stage[field] = false
					}
					centerReply(w, 0, stage)
				default:
					t.Error("mismatched package reached upgrade route")
					w.WriteHeader(404)
				}
			}))
			if err := c.upgrade(context.Background(), "/verified.fpk", "1.3.16", func(string) error { return nil }); err == nil {
				t.Fatal("accepted mismatched package")
			}
		})
	}
}

func TestCenterHTTP200DoesNotHideBusinessFailure(t *testing.T) {
	for _, body := range []string{`{"code":19000,"msg":"refused"}`, `{"data":{}}`, `{"code":null}`, `invalid JSON`} {
		t.Run(body, func(t *testing.T) {
			c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) }))
			if err := c.call(context.Background(), "/rpc/v1/update/task", map[string]any{}, nil); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}

func TestCenterReadOnlyCapabilityProbe(t *testing.T) {
	c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if r.URL.Path != "/rpc/v1/update/info" || len(payload) != 0 {
			t.Errorf("probe: %s %#v", r.URL.Path, payload)
		}
		centerReply(w, 10030, nil)
	}))
	if err := c.available(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCenterCompletionNeedsInstalledVersion(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          int
		version         string
		failed, unknown bool
	}{
		{"completed_target", 2, "1.3.16", false, false},
		{"completed_old", 2, "1.3.15", true, true},
		{"reaped_target", 5, "1.3.16", false, false},
		{"reaped_old", 5, "1.3.15", true, true},
		{"explicit_failure", 3, "1.3.15", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				centerReply(w, 0, map[string]any{"status": tc.status, "message": "task failed"})
			}))
			c.installedVersion = func() (string, error) { return tc.version, nil }
			err := c.waitUpgrade(context.Background(), "task", "1.3.16")
			if (err != nil) != tc.failed || errors.Is(err, errUpgradeUnknown) != tc.unknown {
				t.Fatalf("result = %v", err)
			}
		})
	}
}

func TestCenterUncertainSubmissionIsNeverRetried(t *testing.T) {
	var submissions atomic.Int32
	c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/v1/download/task":
			centerReply(w, 0, map[string]any{"downloadTaskId": "one"})
		case "/rpc/v1/download/status":
			centerReply(w, 0, readyStage())
		case "/rpc/v1/update/info":
			centerReply(w, 0, map[string]any{"wizardInfo": map[string]any{"installedVolumeID": 1}})
		case "/rpc/v1/update/task":
			submissions.Add(1)
			w.Write([]byte("response lost"))
		default:
			t.Errorf("unexpected fallback: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	err := c.upgrade(context.Background(), "/verified.fpk", "1.3.16", func(string) error { return nil })
	if !errors.Is(err, errUpgradeUnknown) || submissions.Load() != 1 {
		t.Fatalf("result=%v submissions=%d", err, submissions.Load())
	}
}

func TestCenterRejectsMissingVolumeOrUpgradeWizard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		volume int
		wizard bool
	}{{"missing_volume", 0, false}, {"wizard", 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeCenter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rpc/v1/download/task":
					centerReply(w, 0, map[string]any{"downloadTaskId": "one"})
				case "/rpc/v1/download/status":
					centerReply(w, 0, readyStage())
				case "/rpc/v1/update/info":
					centerReply(w, 0, map[string]any{"wizardInfo": map[string]any{"installedVolumeID": tc.volume, "hasWizard": tc.wizard}})
				default:
					t.Error("upgrade submitted without valid preflight")
					w.WriteHeader(404)
				}
			}))
			if err := c.upgrade(context.Background(), "/verified.fpk", "1.3.16", func(string) error { return nil }); err == nil {
				t.Fatal("invalid preflight accepted")
			}
		})
	}
}
