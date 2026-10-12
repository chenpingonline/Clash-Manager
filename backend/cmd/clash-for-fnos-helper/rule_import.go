package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/chenpingonline/Clash-Manager/backend/internal/ruleimport"
)

// Only data-only MRS assets are accepted. The selected Core determines the
// directory; callers cannot choose a file name, destination or executable.
func (h *helper) saveRuleAsset(body map[string]any) (map[string]any, error) {
	status, err := h.geoStatus()
	if err != nil {
		return nil, err
	}
	home, _ := status["homeDir"].(string)
	if !filepath.IsAbs(home) || home != stringField(body, "homeDir") {
		return nil, fail(409, "内核数据目录已变化，请重新预览")
	}
	// External Core integration is read-only; it can still consume remote MRS.
	if status["readOnly"] == true {
		return nil, fail(409, "外部 Core 的本地 MRS 请在其数据目录中管理，或通过链接导入")
	}
	data, err := base64.StdEncoding.DecodeString(stringField(body, "content"))
	if err != nil {
		return nil, fail(400, "MRS 编码无效")
	}
	parsed, err := ruleimport.Parse(data, ruleimport.Options{}, nil)
	if err != nil || parsed.Format != "MRS" {
		return nil, fail(400, "MRS 文件校验失败")
	}
	sum := sha256.Sum256(data)
	directory := filepath.Join(home, "rule-import")
	path := filepath.Join(directory, hex.EncodeToString(sum[:])+".mrs")
	if stat, err := os.Lstat(directory); err == nil {
		if !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
			return nil, fail(400, "规则文件目录无效")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0750); err != nil {
		return nil, err
	}
	if stat, err := os.Lstat(path); err == nil {
		if !stat.Mode().IsRegular() {
			return nil, fail(400, "规则文件目标无效")
		}
		old, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(old, data) {
			return nil, fail(409, "同名 MRS 文件内容不匹配")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if err := atomicWrite(path, data, 0644); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "path": path}, nil
}
