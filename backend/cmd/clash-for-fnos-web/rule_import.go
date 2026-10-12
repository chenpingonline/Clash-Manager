package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/chenpingonline/Clash-Manager/backend/internal/ruleimport"
)

type ruleImportPreview struct {
	ID          string `json:"token"`
	ProfileID   string `json:"profileId"`
	ExpiresAt   int64  `json:"expiresAt"`
	Fingerprint string `json:"-"`
	HomeDir     string `json:"-"`
	*ruleimport.Plan
}
type ruleImportInput struct {
	Source   string `json:"source"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	ruleimport.Options
}

func (g *gateway) ruleImportSnapshot(id string) ([]byte, string, map[string]string, map[string]string, string, error) {
	state, err := g.readProfiles()
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	item := findProfile(&state, id)
	if item == nil {
		return nil, "", nil, nil, "", errors.New("配置不存在")
	}
	raw, err := os.ReadFile(filepath.Join(g.config.profileDir, id+".yaml"))
	if err != nil {
		return nil, "", nil, nil, "", errors.New("请先下载或导入订阅配置，再导入规则集")
	}
	existing, err := g.readProfileExtensions(id)
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	global, err := g.readProfileExtensions("_global")
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	fingerprintData, _ := json.Marshal([]any{item.Name, string(raw), existing, global})
	sum := sha256.Sum256(fingerprintData)
	return raw, item.Name, existing, global, hex.EncodeToString(sum[:]), nil
}
func (g *gateway) ruleImportHome(ctx context.Context) (string, error) {
	if g.config.privilegedSocket != "" {
		status, err := g.geoStatus(ctx)
		if err != nil {
			return "", errors.New("无法确认内核数据目录，请先检查 Core 状态")
		}
		if status["readOnly"] == true {
			return "", errors.New("外部 Core 的本地 MRS 请在其数据目录中管理，或通过链接导入")
		}
		home, _ := status["homeDir"].(string)
		if !filepath.IsAbs(home) {
			return "", errors.New("内核数据目录无效")
		}
		return home, nil
	}
	return filepath.Join(filepath.Dir(g.config.profileDir), "mihomo"), nil
}
func (g *gateway) handleRuleImportAPI(w http.ResponseWriter, r *http.Request, id, operation string) bool {
	if r.Method != http.MethodPost || (operation != "preview" && operation != "save") {
		return false
	}
	if operation == "save" {
		g.saveRuleImport(w, r, id)
		return true
	}
	var input ruleImportInput
	if !decodeJSONBody(w, r, &input) {
		return true
	}
	if input.Placement != "" && input.Placement != "auto" && input.Placement != "prepend" && input.Placement != "append" && input.Placement != "replace" {
		writeJSON(w, 400, map[string]string{"error": "规则插入位置无效"})
		return true
	}
	g.profileMu.Lock()
	raw, name, existing, global, fingerprint, err := g.ruleImportSnapshot(id)
	g.profileMu.Unlock()
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	budget := 32 << 20
	fetch := func(rawURL string) ([]byte, error) {
		u, err := ruleimport.URL(rawURL)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Clash-Manager/rule-import")
		client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("重定向过多")
			}
			_, err := ruleimport.URL(req.URL.String())
			return err
		}}
		response, err := client.Do(req)
		if err != nil {
			return nil, errors.New("规则链接下载失败，请检查地址及网络")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("规则链接返回 HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, ruleimport.MaxInput+1))
		if err != nil {
			return nil, errors.New("读取规则内容失败")
		}
		budget -= len(data)
		if len(data) > ruleimport.MaxInput || budget < 0 {
			return nil, errors.New("规则下载超过大小限制")
		}
		return data, nil
	}
	var data []byte
	switch input.Source {
	case "url":
		u, e := ruleimport.URL(input.URL)
		if e != nil {
			err = e
		} else {
			input.URL = u
			data, err = fetch(u)
		}
	case "file", "text":
		input.URL = ""
		if input.Encoding == "base64" && input.Source == "file" {
			data, err = base64.StdEncoding.DecodeString(input.Content)
		} else if input.Encoding == "" {
			data = []byte(input.Content)
		} else {
			err = errors.New("导入编码无效")
		}
	default:
		err = errors.New("请选择文件、链接或粘贴文本")
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return true
	}
	parsed, err := ruleimport.Parse(data, input.Options, fetch)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return true
	}
	home := ""
	for _, p := range parsed.Providers {
		if len(p.Asset) > 0 {
			home, err = g.ruleImportHome(ctx)
			break
		}
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return true
	}
	plan, err := ruleimport.Build(parsed, input.Options, raw, name, global, existing, func(data []byte) string {
		sum := sha256.Sum256(data)
		return filepath.Join(home, "rule-import", hex.EncodeToString(sum[:])+".mrs")
	})
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return true
	}
	preview := &ruleImportPreview{ID: newHexID(24), ProfileID: id, Fingerprint: fingerprint, ExpiresAt: time.Now().Add(15 * time.Minute).UnixMilli(), HomeDir: home, Plan: plan}
	g.profileMu.Lock()
	defer g.profileMu.Unlock()
	if g.ruleImportPreviews == nil {
		g.ruleImportPreviews = map[string]*ruleImportPreview{}
	}
	for key, p := range g.ruleImportPreviews {
		if p.ExpiresAt < time.Now().UnixMilli() || p.ProfileID == id {
			delete(g.ruleImportPreviews, key)
		}
	}
	if len(g.ruleImportPreviews) >= 8 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "导入预览过多，请稍后重试"})
		return true
	}
	g.ruleImportPreviews[preview.ID] = preview
	// Include a bounded rule sample; the exact proposed enhancement YAML is also available.
	view := *preview
	viewPlan := *plan
	viewResult := *parsed
	if len(viewResult.Rules) > 100 {
		viewResult.Rules = viewResult.Rules[:100]
	}
	viewPlan.Result = &viewResult
	view.Plan = &viewPlan
	writeJSON(w, http.StatusOK, view)
	return true
}
func (g *gateway) saveRuleImport(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Token       string `json:"token"`
		Acknowledge bool   `json:"acknowledge"`
		Apply       bool   `json:"apply"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	g.profileMu.Lock()
	defer g.profileMu.Unlock()
	preview := g.ruleImportPreviews[body.Token]
	if preview == nil || preview.ProfileID != id || preview.ExpiresAt < time.Now().UnixMilli() {
		writeJSON(w, 409, map[string]string{"error": "导入预览已过期，请重新预览"})
		return
	}
	if len(preview.Errors) > 0 {
		writeJSON(w, 400, map[string]string{"error": "预览中存在错误，请修正后重新导入"})
		return
	}
	if len(preview.Warnings) > 0 && !body.Acknowledge {
		writeJSON(w, 400, map[string]string{"error": "请确认已阅读导入提示及未导入的条目"})
		return
	}
	g.jobMu.Lock()
	active := g.profileJobs[g.activeJobs[id]]
	running := active != nil && active.State == "running"
	g.jobMu.Unlock()
	if running {
		writeJSON(w, 409, map[string]string{"error": "此订阅正在更新或应用，请完成后重新保存"})
		return
	}
	_, _, _, _, fingerprint, err := g.ruleImportSnapshot(id)
	if err != nil || fingerprint != preview.Fingerprint {
		writeJSON(w, 409, map[string]string{"error": "订阅或增强配置已变化，请重新预览"})
		return
	}
	if preview.HomeDir != "" {
		home, err := g.ruleImportHome(r.Context())
		if err != nil || home != preview.HomeDir {
			writeJSON(w, 409, map[string]string{"error": "内核数据目录已变化，请重新预览"})
			return
		}
	}
	// Validate every output before beginning the reversible write transaction.
	files := map[string][]byte{}
	for kind, content := range preview.Extensions {
		if err := configyaml.ValidateExtension(kind, content); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		files[filepath.Join(g.config.profileDir, id+"."+configyaml.ExtensionKinds[kind])] = []byte(content)
	}
	for path, asset := range preview.Assets {
		if g.config.privilegedSocket == "" {
			files[path] = asset
			continue
		}
		var result map[string]any
		if err := g.helperJSON(r.Context(), http.MethodPost, "/config/rule-asset", map[string]any{"homeDir": preview.HomeDir, "content": base64.StdEncoding.EncodeToString(asset)}, &result, 15*time.Second); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if result["path"] != path {
			writeJSON(w, 409, map[string]string{"error": "内核数据目录已变化，请重新预览"})
			return
		}
	}
	if err := writeRuleImportFiles(files); err != nil {
		writeJSON(w, 500, map[string]string{"error": "保存规则增强失败: " + err.Error()})
		return
	}
	delete(g.ruleImportPreviews, body.Token)
	if body.Apply {
		job := g.startProfileJobLocked(id, "activate")
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "profileId": id, "applied": false})
}

// Roll back previous files if a later file cannot be written. The content-addressed
// MRS assets are written first; existing enhancements are never partially replaced.
func writeRuleImportFiles(files map[string][]byte) error {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		mi, mj := strings.HasSuffix(paths[i], ".mrs"), strings.HasSuffix(paths[j], ".mrs")
		if mi != mj {
			return mi
		}
		return paths[i] < paths[j]
	})
	snapshots := map[string][]byte{}
	exists := map[string]bool{}
	written := []string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		snapshots[path] = data
		exists[path] = err == nil
	}
	for _, path := range paths {
		if err := writeAtomicFile(path, files[path]); err != nil {
			for i := len(written) - 1; i >= 0; i-- {
				name := written[i]
				var rollback error
				if exists[name] {
					rollback = writeAtomicFile(name, snapshots[name])
				} else {
					rollback = os.Remove(name)
				}
				if rollback != nil {
					err = errors.Join(err, fmt.Errorf("恢复原文件失败: %w", rollback))
				}
			}
			return err
		}
		written = append(written, path)
	}
	return nil
}
