package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/runtimeenv"
)

// Probes never allocate an interface or change routes. Repair is a separate,
// explicit operation with fixed commands and paths; request data is not executed.
type tunProbe struct {
	Device bool   `json:"device"`
	Driver bool   `json:"driver"`
	Usable bool   `json:"usable"`
	Error  string `json:"error,omitempty"`
}
type tunRepairOps struct {
	probe  func() tunProbe
	load   func(context.Context) error
	create func() error
}

var tunRepairMu sync.Mutex

func probeTun() tunProbe {
	p := tunProbe{}
	misc, _ := os.ReadFile("/proc/misc")
	for _, line := range strings.Split(string(misc), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "200" && fields[1] == "tun" {
			p.Driver = true
		}
	}
	info, err := os.Lstat("/dev/net/tun")
	if os.IsNotExist(err) {
		return p
	}
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.Device = true
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeCharDevice == 0 {
		p.Error = "TUN path is not a character device"
		return p
	}
	if err = probeTunDevice(); err != nil {
		p.Error = err.Error()
		return p
	}
	p.Usable, p.Driver = true, true
	return p
}

func (h *helper) tunDiagnostics() map[string]any {
	p := probeTun()
	proc := h.primary()
	capability := resolveTunCapability(proc, p.Device, os.Geteuid())
	if runtime.GOOS == "linux" {
		// Check the actual Core capability rather than inferring it from Helper UID.
		if proc != nil {
			status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", proc.PID))
			capability["permission"] = err == nil && hasNetAdmin(string(status))
		}
	}
	permission, _ := capability["permission"].(bool)
	capability["supported"] = p.Usable && permission
	switch {
	case runtime.GOOS != "linux":
		capability["reason"], capability["message"] = "unsupported-platform", "TUN diagnostics require Linux"
	case !p.Device && !p.Driver:
		capability["reason"], capability["message"] = "tun-driver-unavailable", "未检测到 TUN 设备与已加载驱动，可尝试修复；虚拟机需确认宿主支持 TUN"
	case !p.Device:
		capability["reason"], capability["message"] = "tun-device-missing", "TUN 驱动已加载，但缺少 /dev/net/tun，可尝试修复设备节点"
	case !p.Usable:
		capability["reason"], capability["message"] = "tun-device-unusable", "TUN 设备无法访问，请检查设备权限或容器设备透传"
	case proc == nil:
		capability["reason"], capability["message"] = "core-not-running", "当前未检测到运行中的 Mihomo Core"
	case !permission:
		capability["reason"], capability["message"] = "permission-denied", "当前 Mihomo 缺少 TUN 所需的 NET_ADMIN 权限"
	}
	kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	repairable := runtime.GOOS == "linux" && !runtimeenv.Docker() && os.Geteuid() == 0 && !p.Usable && p.Error == ""
	return map[string]any{"capability": capability, "probe": p, "platform": runtimeenv.Platform(), "os": runtime.GOOS, "arch": runtime.GOARCH, "kernel": strings.TrimSpace(string(kernel)), "repairable": repairable}
}

func repairTun(ctx context.Context, allowed bool, ops tunRepairOps) ([]string, error) {
	if !allowed {
		return nil, fail(409, "当前环境不能自动修复 TUN，请在宿主系统检查驱动、设备透传与权限")
	}
	steps := []string{}
	p := ops.probe()
	if p.Usable {
		return steps, nil
	}
	if p.Error != "" {
		return steps, fmt.Errorf("TUN 设备检查失败: %s", p.Error)
	}
	if !p.Driver {
		if err := ops.load(ctx); err != nil {
			return steps, fmt.Errorf("加载 TUN 驱动失败，请检查内核支持或虚拟机设备透传: %w", err)
		}
		steps = append(steps, "module-loaded")
		p = ops.probe()
	}
	if !p.Driver {
		return steps, fmt.Errorf("内核未注册 TUN 驱动，请检查宿主系统支持")
	}
	if !p.Device {
		if err := ops.create(); err != nil {
			return steps, fmt.Errorf("创建 TUN 设备节点失败: %w", err)
		}
		steps = append(steps, "device-created")
	}
	p = ops.probe()
	if !p.Usable {
		return steps, fmt.Errorf("TUN 修复后仍不可用，请检查设备权限或宿主透传: %s", p.Error)
	}
	return steps, nil
}
func loadTunModule(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for _, path := range []string{"/usr/sbin/modprobe", "/sbin/modprobe"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			output, err := exec.CommandContext(ctx, path, "tun").CombinedOutput()
			if err != nil {
				return fmt.Errorf("%w: %.1024s", err, output)
			}
			return nil
		}
	}
	return fmt.Errorf("modprobe is not installed")
}
func (h *helper) repairTun(ctx context.Context) map[string]any {
	tunRepairMu.Lock()
	defer tunRepairMu.Unlock()
	before := h.tunDiagnostics()
	allowed, _ := before["repairable"].(bool)
	steps, err := repairTun(ctx, allowed, tunRepairOps{probeTun, loadTunModule, createTunDevice})
	result := h.tunDiagnostics()
	result["steps"] = steps
	result["ok"] = err == nil
	if err != nil {
		result["error"] = err.Error()
	}
	// This operation never enables TUN or edits any Core configuration.
	return result
}
