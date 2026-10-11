package appupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const StateDir = "/var/tmp/clash-for-fnos-update"
const UnitName = "clash-for-fnos-update"

type Status struct {
	Stage           string `json:"stage"`
	TargetVersion   string `json:"targetVersion,omitempty"`
	Progress        int    `json:"progress"`
	Downloaded      int64  `json:"downloaded,omitempty"`
	Total           int64  `json:"total,omitempty"`
	Error           string `json:"error,omitempty"`
	UpdatedAt       int64  `json:"updatedAt"`
	AppCenterTaskID string `json:"appCenterTaskId,omitempty"`
	OutcomeUnknown  bool   `json:"outcomeUnknown,omitempty"`
}

func (s Status) Active() bool {
	return s.Stage == "queued" || s.Stage == "downloading" || s.Stage == "verifying" || s.Stage == "installing" || s.Stage == "restarting"
}

func Prepare(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("更新工作目录不安全")
	}
	// Checking ownership without syscall-specific structures also works in the
	// macOS build used by developers. Chown to the current root uid must succeed.
	if os.Geteuid() == 0 {
		// A pre-existing directory must be root-owned; never adopt an attacker's files.
		if !rootOwned(info) {
			return errors.New("更新工作目录必须由 root 拥有")
		}
		if info.Mode().Perm()&0o077 != 0 {
			return errors.New("更新工作目录权限不安全")
		}
	}
	return os.Chmod(dir, 0o700)
}

func ReadStatus(dir string) (Status, error) {
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if os.IsNotExist(err) {
		return Status{Stage: "idle"}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var status Status
	err = json.Unmarshal(data, &status)
	return status, err
}

func WriteStatus(dir string, status Status) error {
	status.UpdatedAt = time.Now().UnixMilli()
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".status-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filepath.Join(dir, "status.json"))
}

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}

func trustedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "api.github.com" || host == "github.com" || host == "githubusercontent.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

func Client() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &http.Client{Transport: t, Timeout: 20 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !trustedURL(req.URL.String()) {
			return errors.New("更新下载重定向地址不受信任")
		}
		return nil
	}}
}

func resolve(ctx context.Context, client *http.Client, target, arch string) (asset, error) {
	if !ValidRepository(Repository) {
		return asset{}, errors.New("构建时的更新仓库配置无效")
	}
	v, err := Version(target)
	if err != nil {
		return asset{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+Repository+"/releases/tags/v"+v, nil)
	req.Header.Set("User-Agent", "Clash-for-fnos-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(req)
	if err != nil {
		return asset{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return asset{}, fmt.Errorf("获取更新版本失败: HTTP %d", response.StatusCode)
	}
	var r release
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&r); err != nil {
		return asset{}, err
	}
	if r.Tag != "v"+v || r.Draft || r.Prerelease {
		return asset{}, errors.New("更新必须为公开稳定版本")
	}
	suffix := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[arch]
	if suffix == "" {
		return asset{}, errors.New("不支持当前架构")
	}
	for _, a := range r.Assets {
		if a.Name != "Clash.for.fnos_"+v+"_"+suffix+".fpk" && a.Name != "Clash for fnos_"+v+"_"+suffix+".fpk" {
			continue
		}
		u, err := url.Parse(a.URL)
		if err != nil || !trustedURL(a.URL) || u.Hostname() != "github.com" || !strings.HasPrefix(u.Path, "/"+Repository+"/releases/download/v"+v+"/") || a.Size <= 0 || a.Size > MaxPackageSize {
			return asset{}, errors.New("更新资源信息无效")
		}
		if !strings.HasPrefix(a.Digest, "sha256:") || len(a.Digest) != 71 {
			return asset{}, errors.New("更新资源缺少 SHA-256 校验信息")
		}
		return a, nil
	}
	return asset{}, errors.New("此版本没有对应架构的 FPK")
}

// Download writes only to a root-owned directory. It rejects both truncated
// responses and data beyond the size advertised by GitHub.
func Download(ctx context.Context, client *http.Client, a asset, destination string, progress func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Clash-for-fnos-updater")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新失败: HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != a.Size {
		return errors.New("更新包大小与发布信息不一致")
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := io.LimitReader(response.Body, a.Size+1)
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if _, err = f.Write(buffer[:n]); err != nil {
				return err
			}
			total += int64(n)
			progress(total)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if total != a.Size {
		return errors.New("更新包下载不完整或超过大小限制")
	}
	return f.Sync()
}

// Worker is run by a transient systemd unit, independently of the application's
// main/helper processes. App Center can safely stop and replace those processes.
func Worker(target, current, varDir string) (resultErr error) {
	if os.Geteuid() != 0 || runtime.GOOS != "linux" {
		return errors.New("应用更新任务必须在飞牛设备上以 root 运行")
	}
	if err := Prepare(StateDir); err != nil {
		return err
	}
	unlock, err := lockWorker(StateDir)
	if err != nil {
		return err
	}
	defer unlock()
	status := Status{Stage: "queued", TargetVersion: target}
	defer func() {
		if resultErr != nil {
			status.Stage = "error"
			status.Error = resultErr.Error()
			status.OutcomeUnknown = errors.Is(resultErr, errUpgradeUnknown)
			_ = WriteStatus(StateDir, status)
		}
	}()
	if !Newer(target, current) {
		return errors.New("目标版本必须高于当前版本")
	}
	// Logs belong to this task. Installation and recovery must not overwrite
	// each other's evidence or leave a previous task's output behind.
	for _, name := range []string{"install.log", "start.log", "health.log"} {
		if err := os.Remove(filepath.Join(StateDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if err := CheckUpgradeAPI(ctx); err != nil {
		return err
	}
	client := Client()
	defer client.CloseIdleConnections()
	a, err := resolve(ctx, client, target, runtime.GOARCH)
	if err != nil {
		return err
	}
	status.Stage = "downloading"
	status.Total = a.Size
	if err = WriteStatus(StateDir, status); err != nil {
		return err
	}
	filename := filepath.Join(StateDir, "update.fpk")
	_ = os.Remove(filename)
	defer os.Remove(filename)
	last := time.Time{}
	err = Download(ctx, client, a, filename, func(n int64) {
		status.Downloaded = n
		status.Progress = int(n * 80 / a.Size)
		if time.Since(last) > 500*time.Millisecond {
			_ = WriteStatus(StateDir, status)
			last = time.Now()
		}
	})
	if err != nil {
		return err
	}
	var healthDetail string
	err = installVerified(ctx, filename, a.Digest, target, runtime.GOARCH,
		func(stage string, progress int) error {
			status.Stage = stage
			status.Progress = progress
			status.Error = ""
			return WriteStatus(StateDir, status)
		},
		func() error {
			return upgradeAppCenter(ctx, filename, target, func(id string) error {
				status.AppCenterTaskID = id
				return WriteStatus(StateDir, status)
			})
		},
		func() error { return runCLI(ctx, "start", AppName) },
		func(timeout time.Duration) bool {
			var healthy bool
			healthy, healthDetail = waitHealthy(ctx, varDir, target, timeout)
			_ = os.WriteFile(filepath.Join(StateDir, "health.log"), []byte(healthDetail+"\n"), 0o600)
			return healthy
		})
	if err != nil && status.Stage == "restarting" && healthDetail != "" {
		return fmt.Errorf("%w（%s）", err, healthDetail)
	}
	return err
}

// installVerified centralizes the security boundary: no App Center command runs
// before archive verification, and success requires the new gateway's health.
func installVerified(ctx context.Context, filename, digest, target, arch string, save func(string, int) error, install, start func() error, healthy func(time.Duration) bool) error {
	if err := save("verifying", 82); err != nil {
		return err
	}
	if err := Verify(filename, digest, target, arch); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := save("installing", 88); err != nil {
		return err
	}
	if err := install(); err != nil {
		return err
	}
	if err := save("restarting", 95); err != nil {
		return err
	}
	if !healthy(10 * time.Second) {
		if err := start(); err != nil && !healthy(5*time.Second) {
			return err
		}
		if !healthy(60 * time.Second) {
			return errors.New("新版应用未恢复运行，请在飞牛应用中心检查状态")
		}
	}
	return save("success", 100)
}

type limitedOutput struct{ body []byte }

func (w *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(w.body) < 64<<10 {
		remaining := (64 << 10) - len(w.body)
		if len(p) > remaining {
			p = p[:remaining]
		}
		w.body = append(w.body, p...)
	}
	return n, nil
}

func runCLI(ctx context.Context, args ...string) error {
	return runCLIIn(ctx, StateDir, args...)
}

func runCLIIn(ctx context.Context, dir string, args ...string) error {
	if len(args) == 0 || (args[0] != "install-fpk" && args[0] != "start") {
		return errors.New("不支持的飞牛应用操作")
	}
	cmd := exec.CommandContext(ctx, "appcenter-cli", args...)
	var output limitedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	logName := "install.log"
	if args[0] == "start" {
		logName = "start.log"
	}
	logErr := os.WriteFile(filepath.Join(dir, logName), output.body, 0o600)
	// Some command-line tools report a business error on stdout while exiting
	// successfully. An explicit error line must never advance to restart.
	if err == nil && cliReportedError(output.body) {
		err = errors.New("应用中心返回错误")
	}
	if err != nil {
		message := strings.TrimSpace(string(output.body))
		if len(message) > 2048 {
			message = message[:2048]
		}
		return fmt.Errorf("飞牛应用操作失败: %v %s", err, message)
	}
	return logErr
}

func cliReportedError(output []byte) bool {
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "[error]") {
			return true
		}
	}
	return false
}

func waitHealthy(ctx context.Context, varDir, target string, timeout time.Duration) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", filepath.Join(varDir, "clash-for-fnos.sock"))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	detail := "健康检查未完成"
	for ctx.Err() == nil {
		version, err := probeHealth(ctx, client, target)
		if err == nil {
			return true, "运行版本 " + version
		}
		if ctx.Err() == nil {
			detail = err.Error()
		}
		select {
		case <-ctx.Done():
			return false, detail
		case <-time.After(time.Second):
		}
	}
	return false, detail
}

func probeHealth(ctx context.Context, client *http.Client, target string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/api/health", nil)
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("无法连接应用健康接口: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("应用健康接口返回 HTTP %d", response.StatusCode)
	}
	var health struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&health); err != nil {
		return "", errors.New("应用健康接口返回无效数据")
	}
	if !health.OK {
		return "", errors.New("应用健康接口尚未就绪")
	}
	v, err := Version(health.Version)
	if err != nil {
		return "", errors.New("应用健康接口没有有效版本")
	}
	if v != strings.TrimPrefix(target, "v") {
		return v, fmt.Errorf("当前运行版本 %s，目标版本 %s", v, strings.TrimPrefix(target, "v"))
	}
	return v, nil
}
