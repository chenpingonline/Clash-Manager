package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/appupdate"
	"github.com/chenpingonline/Clash-Manager/backend/internal/runtimeenv"
)

func appUpdateAvailability() (bool, string) {
	if runtimeenv.Platform() != "fnos" || runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return false, "应用内安装更新仅支持飞牛原生应用"
	}
	for _, name := range []string{"appcenter-cli", "systemd-run", "systemctl"} {
		if _, err := exec.LookPath(name); err != nil {
			return false, "系统缺少更新工具: " + name
		}
	}
	return true, ""
}

func (h *helper) appUpdateStatus() (map[string]any, error) {
	h.appUpdateMu.Lock()
	defer h.appUpdateMu.Unlock()
	available, reason := appUpdateAvailability()
	status := appupdate.Status{Stage: "idle"}
	if available {
		if err := appupdate.Prepare(appupdate.StateDir); err != nil {
			return nil, err
		}
		var err error
		status, err = appupdate.ReadStatus(appupdate.StateDir)
		if err != nil {
			return nil, err
		}
		// A power loss or systemd timeout leaves the last persisted stage behind.
		// Check the unit after a grace period so a new helper does not mistake
		// the expected stop/restart gap for an interrupted task.
		if status.Active() && time.Since(time.UnixMilli(status.UpdatedAt)) > time.Minute {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err = exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", appupdate.UnitName+".service").Run()
			contextErr := ctx.Err()
			cancel()
			if err != nil && contextErr == nil {
				status.Stage = "error"
				status.Error = "更新任务已中断，请重试或在飞牛应用中心检查状态"
				_ = appupdate.WriteStatus(appupdate.StateDir, status)
			}
		}
	}
	return map[string]any{"supported": available, "reason": reason, "status": status}, nil
}

func appUpdateCommand(worker, target, current, varDir string) *exec.Cmd {
	return exec.Command("systemd-run", "--quiet", "--collect", "--unit="+appupdate.UnitName, "--property=Type=exec", "--property=RuntimeMaxSec=1800", "--property=UMask=0077", "--", worker, "--app-update-worker", target, current, varDir)
}

func (h *helper) startAppUpdate(body map[string]any) (any, error) {
	h.appUpdateMu.Lock()
	defer h.appUpdateMu.Unlock()
	if supported, reason := appUpdateAvailability(); !supported {
		return nil, errors.New(reason)
	}
	target, err := appupdate.Version(stringField(body, "version"))
	if err != nil {
		return nil, err
	}
	if !appupdate.Newer(target, version) {
		return nil, errors.New("目标版本必须高于当前版本")
	}
	if err = appupdate.Prepare(appupdate.StateDir); err != nil {
		return nil, err
	}
	previous, err := appupdate.ReadStatus(appupdate.StateDir)
	if err != nil {
		return nil, err
	}
	if previous.Active() {
		return nil, errors.New("已有更新任务正在运行")
	}
	// Check systemd as well: the successful worker can still be exiting while
	// its persisted result is already readable.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	activeErr := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", appupdate.UnitName+".service").Run()
	cancel()
	if activeErr == nil {
		return nil, errors.New("已有更新任务正在运行")
	}
	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.Canceled) {
		return nil, errors.New("无法确认系统更新任务状态")
	}
	source, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Run a root-owned snapshot so replacing the installed helper cannot change
	// the worker executable. Neither path nor command comes from the Web request.
	worker := filepath.Join(appupdate.StateDir, "worker")
	input, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	f, err := os.CreateTemp(appupdate.StateDir, ".worker-")
	if err != nil {
		return nil, err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	_, err = io.Copy(f, input)
	if err == nil {
		err = f.Chmod(0o700)
	}
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err = os.Rename(temporary, worker); err != nil {
		return nil, err
	}
	status := appupdate.Status{Stage: "queued", TargetVersion: target}
	if err = appupdate.WriteStatus(appupdate.StateDir, status); err != nil {
		return nil, err
	}
	cmd := appUpdateCommand(worker, target, version, h.config.varDir)
	// The manager starts the independent unit; do not attach its lifetime to
	// this HTTP request or to the helper's shutdown context.
	output, err := cmd.CombinedOutput()
	if err != nil {
		status.Stage = "error"
		status.Error = fmt.Sprintf("无法启动更新任务: %v %s", err, strings.TrimSpace(string(output)))
		_ = appupdate.WriteStatus(appupdate.StateDir, status)
		return nil, errors.New(status.Error)
	}
	return map[string]any{"supported": true, "status": status}, nil
}
