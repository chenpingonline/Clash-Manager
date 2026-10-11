package appupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const AppCenterSocket = "/var/run/com.trim.app.center.sock"

// These are the local daemon's RPC routes, rather than the browser-facing
// /app-center routes. The privileged worker only upgrades this application's
// already-verified FPK; paths and application identity never come from HTTP.
type appCenter struct {
	client           *http.Client
	pollInterval     time.Duration
	log              io.Writer
	installedVersion func() (string, error)
}

type centerError struct {
	code    int
	message string
}

func (e *centerError) Error() string {
	return fmt.Sprintf("飞牛应用中心返回错误 %d: %s", e.code, e.message)
}

var errUpgradeUnknown = errors.New("应用中心升级结果尚未确认，请先在应用中心检查任务，勿重复提交")

func newAppCenter(socket string) *appCenter {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	return &appCenter{
		client:       &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("应用中心接口不允许重定向") }},
		pollInterval: time.Second,
		installedVersion: func() (string, error) {
			body, err := os.ReadFile("/var/apps/" + AppName + "/manifest")
			if err != nil {
				return "", err
			}
			manifest, err := Manifest(string(body))
			if err != nil {
				return "", err
			}
			if manifest["appname"] != AppName {
				return "", errors.New("已安装应用身份不匹配")
			}
			return Version(manifest["version"])
		},
	}
}

func (c *appCenter) call(ctx context.Context, route string, payload any, out any) (resultErr error) {
	defer func() {
		if c.log != nil {
			_, _ = fmt.Fprintf(c.log, "%s %s", time.Now().Format(time.RFC3339), route)
			if resultErr != nil {
				_, _ = fmt.Fprintf(c.log, " error=%v", resultErr)
			}
			_, _ = fmt.Fprintln(c.log)
		}
	}()
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost"+route, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("应用中心接口 %s 返回 HTTP %d", route, response.StatusCode)
	}
	var envelope struct {
		Code    *int            `json:"code"`
		Message string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("应用中心接口返回无效数据: %w", err)
	}
	if envelope.Code == nil {
		return errors.New("应用中心接口没有返回业务状态码")
	}
	if *envelope.Code != 0 {
		return &centerError{code: *envelope.Code, message: envelope.Message}
	}
	if out != nil {
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			return errors.New("应用中心接口没有返回任务数据")
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("应用中心任务数据无效: %w", err)
		}
	}
	return nil
}

// The empty info request is read-only and must return the daemon's validation
// error. This verifies that the real upgrade route exists before downloading.
func (c *appCenter) available(ctx context.Context) error {
	err := c.call(ctx, "/rpc/v1/update/info", map[string]any{}, nil)
	var rejected *centerError
	if errors.As(err, &rejected) && rejected.code == 10030 {
		return nil
	}
	return fmt.Errorf("飞牛应用中心的本机升级接口不可用: %v", err)
}

func CheckUpgradeAPI(ctx context.Context) error {
	c := newAppCenter(AppCenterSocket)
	defer c.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.available(ctx)
}

type centerStage struct {
	Status      int    `json:"status"`
	Message     string `json:"message"`
	AppName     string `json:"appName"`
	Version     string `json:"version"`
	PackageType string `json:"packageType"`
	Installed   bool   `json:"installed"`
}

type centerTask struct {
	Status   int     `json:"status"`
	Message  string  `json:"message"`
	Output   string  `json:"outputText"`
	Progress float64 `json:"progress"`
}

func pausePoll(ctx context.Context, interval time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(interval):
		return nil
	}
}

func (c *appCenter) upgrade(ctx context.Context, filename, target string, recordTask func(string) error) error {
	stageCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var submitted struct {
		ID string `json:"downloadTaskId"`
	}
	if err := c.call(stageCtx, "/rpc/v1/download/task", map[string]any{"packageSourceType": "file", "path": filename}, &submitted); err != nil {
		return fmt.Errorf("应用中心暂存更新包失败: %w", err)
	}
	if submitted.ID == "" {
		return errors.New("应用中心未返回更新包暂存任务 ID")
	}
	if c.log != nil {
		_, _ = fmt.Fprintf(c.log, "downloadTaskId=%q targetVersion=%q\n", submitted.ID, target)
	}
	var staged centerStage
	failures := 0
	for {
		staged = centerStage{}
		err := c.call(stageCtx, "/rpc/v1/download/status", map[string]any{"downloadTaskId": submitted.ID}, &staged)
		if err != nil {
			var failure *centerError
			if errors.As(err, &failure) && failure.code != 10050 {
				return err
			}
			failures++
			if failures >= 5 {
				return fmt.Errorf("无法确认更新包暂存状态: %w", err)
			}
		} else {
			failures = 0
			if staged.Status == 2 {
				break
			}
			if staged.Status != 1 {
				return fmt.Errorf("应用中心暂存更新包失败: 状态 %d %s", staged.Status, staged.Message)
			}
		}
		if err := pausePoll(stageCtx, c.pollInterval); err != nil {
			return fmt.Errorf("更新包暂存超时: %w", err)
		}
	}
	// Recheck the daemon's package identity even though the archive was already
	// verified. Never let a staging mix-up upgrade a different application.
	if c.log != nil {
		data, _ := json.Marshal(staged)
		_, _ = fmt.Fprintf(c.log, "stagedPackage=%s\n", data)
	}
	if staged.AppName != AppName {
		return fmt.Errorf("应用中心识别的应用名 %q 与目标 %q 不匹配", staged.AppName, AppName)
	}
	if strings.TrimPrefix(staged.Version, "v") != target {
		return fmt.Errorf("应用中心识别的版本 %q 与目标 %q 不匹配", staged.Version, target)
	}
	if !staged.Installed {
		return errors.New("应用中心没有将更新目标识别为已安装应用，已中止升级")
	}
	// packageSourceType describes where the FPK came from (a local file).
	// packageType identifies the daemon's parsed package kind (e.g. path).
	// Only the daemon can supply that routing value; these are not the same enum.
	if staged.PackageType == "" || strings.TrimSpace(staged.PackageType) != staged.PackageType {
		return fmt.Errorf("应用中心返回的包类型 %q 无效", staged.PackageType)
	}
	var info struct {
		Wizard struct {
			Volume    int  `json:"installedVolumeID"`
			HasWizard bool `json:"hasWizard"`
		} `json:"wizardInfo"`
	}
	update := map[string]any{"appName": AppName, "updateVersion": target, "packageType": staged.PackageType, "language": "zh-CN"}
	if err := c.call(ctx, "/rpc/v1/update/info", update, &info); err != nil {
		return fmt.Errorf("应用中心升级前检查失败: %w", err)
	}
	if info.Wizard.Volume <= 0 {
		return errors.New("应用中心未能确认现有应用所在存储卷，已中止升级")
	}
	if info.Wizard.HasWizard {
		return errors.New("此版本需要填写升级向导，请通过飞牛应用中心安装")
	}
	update["systemParameters"] = map[string]any{"agreedToProtocol": true, "installVolumeID": info.Wizard.Volume, "dataVolumeId": info.Wizard.Volume, "immediateStart": true}
	update["customParameters"] = []any{}
	var task struct {
		ID string `json:"taskId"`
	}
	if err := c.call(ctx, "/rpc/v1/update/task", update, &task); err != nil {
		var rejected *centerError
		if errors.As(err, &rejected) {
			return err
		}
		return fmt.Errorf("%w: %v", errUpgradeUnknown, err)
	}
	if task.ID == "" {
		return fmt.Errorf("%w: 未返回升级任务 ID", errUpgradeUnknown)
	}
	if err := recordTask(task.ID); err != nil {
		return fmt.Errorf("%w: 无法保存任务 ID: %v", errUpgradeUnknown, err)
	}
	return c.waitUpgrade(ctx, task.ID, target)
}

func (c *appCenter) waitUpgrade(ctx context.Context, id, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	var outage time.Time
	for {
		var task centerTask
		err := c.call(ctx, "/rpc/v1/common/status", map[string]any{"taskId": id}, &task)
		if err != nil {
			if outage.IsZero() {
				outage = time.Now()
			}
			if ctx.Err() != nil || time.Since(outage) > 90*time.Second {
				return fmt.Errorf("%w: %v", errUpgradeUnknown, err)
			}
		} else {
			outage = time.Time{}
			switch task.Status {
			case 1:
			case 2:
				version, err := c.installedVersion()
				if err != nil || version != target {
					return fmt.Errorf("%w: 任务已结束，但已安装版本 %s 与目标 %s 不一致（%v）", errUpgradeUnknown, version, target, err)
				}
				return nil
			case 5:
				version, err := c.installedVersion()
				if err == nil && version == target {
					return nil
				}
				return fmt.Errorf("%w: 升级任务已不可查询，当前安装版本 %s（%v）", errUpgradeUnknown, version, err)
			default:
				detail := task.Message
				if detail == "" {
					detail = task.Output
				}
				return fmt.Errorf("应用中心升级失败: 状态 %d %s", task.Status, detail)
			}
		}
		if err := pausePoll(ctx, c.pollInterval); err != nil {
			return fmt.Errorf("%w: %v", errUpgradeUnknown, err)
		}
	}
}

func upgradeAppCenter(ctx context.Context, filename, target string, recordTask func(string) error) error {
	log, err := os.OpenFile(filepath.Join(StateDir, "install.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	c := newAppCenter(AppCenterSocket)
	c.log = log
	defer c.client.CloseIdleConnections()
	return c.upgrade(ctx, filename, target, recordTask)
}
