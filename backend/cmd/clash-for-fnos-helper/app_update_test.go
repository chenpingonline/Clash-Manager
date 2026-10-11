package main

import (
	"strings"
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/appupdate"
)

func TestAppUpdateLaunchUsesIndependentSystemUnit(t *testing.T) {
	cmd := appUpdateCommand("/var/tmp/clash-for-fnos-update/worker", "1.3.16", "1.3.15", "/vol1/@appdata/clash-for-fnos")
	joined := strings.Join(cmd.Args, " ")
	for _, expected := range []string{"systemd-run", "--unit=" + appupdate.UnitName, "RuntimeMaxSec=1800", "--app-update-worker 1.3.16 1.3.15 /vol1/@appdata/clash-for-fnos"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %s: %s", expected, joined)
		}
	}
	if cmd.Cancel != nil {
		t.Fatal("worker launch tied to caller cancellation")
	}
}
