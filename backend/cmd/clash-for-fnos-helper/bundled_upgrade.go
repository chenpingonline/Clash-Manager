package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Package upgrades alone request this reconciliation; ordinary restarts and
// the explicit online updater do not replace a user-selected Core.
func (h *helper) upgradeBundledLocked(ctx context.Context) (string, error) {
	marker := filepath.Join(h.config.varDir, "bundled-core-upgrade.pending")
	if !fileExists(marker) {
		return "", nil
	}
	finish := func() (string, error) { return "", os.Remove(marker) }
	if h.readMode() == "external" || !fileExists(h.config.managedCore) {
		return finish()
	}
	metaPath := filepath.Join(h.config.appDir, "core", "bundled-core.json")
	if !fileExists(metaPath) {
		return finish()
	} // Universal packages have no bundled Core.
	body, err := os.ReadFile(metaPath)
	if err != nil {
		return "", err
	}
	var meta struct {
		Tag string `json:"tag"`
	}
	if err = json.Unmarshal(body, &meta); err != nil {
		return "", err
	}
	current := readVersion(h.config.managedCore)
	newer, err := bundledVersionNewer(meta.Tag, current)
	if err != nil {
		return "", err
	}
	if !newer {
		return finish()
	}
	backup := filepath.Join(h.config.backupDir, "core", "mihomo-package-"+strings.TrimPrefix(current, "v")+"-"+safeStamp())
	if err = copyFile(h.config.managedCore, backup, 0755); err != nil {
		return "", err
	}
	// fnOS stops the application during upgrade. Also reconcile any surviving
	// managed process before replacing the executable; external processes remain untouched.
	h.stopManagedLocked()
	if err = h.installBundled(); err != nil {
		return "", err
	}
	if fileExists(h.config.managedConfig) {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		output, checkErr := exec.CommandContext(checkCtx, h.config.managedCore, "-t", "-d", h.config.managedConfigDir, "-f", h.config.managedConfig).CombinedOutput()
		if checkErr != nil {
			if restoreErr := copyFile(backup, h.config.managedCore, 0755); restoreErr != nil {
				return "", fmt.Errorf("内置内核配置校验失败且恢复失败: %v: %w", checkErr, restoreErr)
			}
			return "", fmt.Errorf("内置内核配置校验失败，已恢复旧内核: %s: %w", strings.TrimSpace(string(output)), checkErr)
		}
	}
	log.Printf("Package bundled Core upgraded from %s to %s; backup: %s", current, meta.Tag, backup)
	return backup, nil
}

func bundledVersionNewer(bundled, current string) (bool, error) {
	pattern := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
	a, b := pattern.FindStringSubmatch(bundled), pattern.FindStringSubmatch(current)
	if a == nil || b == nil {
		return false, fmt.Errorf("无法比较内核版本，保留已安装内核: bundled=%q current=%q", bundled, current)
	}
	for i := 1; i <= 3; i++ {
		av, ae := strconv.ParseUint(a[i], 10, 64)
		bv, be := strconv.ParseUint(b[i], 10, 64)
		if ae != nil || be != nil {
			return false, fmt.Errorf("内核版本无效")
		}
		if av != bv {
			return av > bv, nil
		}
	}
	return false, nil
}

func (h *helper) bootstrapAfterPackageUpgrade(ctx context.Context) (map[string]any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	backup, upgradeErr := h.upgradeBundledLocked(ctx)
	if upgradeErr != nil {
		log.Printf("Package bundled Core upgrade skipped/failed: %v", upgradeErr)
	}
	result, err := h.ensureBootstrapLocked(ctx, false, "")
	if backup == "" {
		return result, err
	}
	if err != nil {
		h.stopManagedLocked()
		if restoreErr := copyFile(backup, h.config.managedCore, 0755); restoreErr != nil {
			return nil, fmt.Errorf("新版内核启动失败: %v；恢复旧内核失败: %w", err, restoreErr)
		}
		log.Printf("Package bundled Core startup failed; restored previous Core: %v", err)
		return h.ensureBootstrapLocked(ctx, false, "")
	}
	if removeErr := os.Remove(filepath.Join(h.config.varDir, "bundled-core-upgrade.pending")); removeErr != nil {
		log.Printf("Failed to clear bundled Core upgrade marker: %v", removeErr)
	}
	return result, nil
}
