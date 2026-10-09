package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chenpingonline/Clash-Manager/backend/internal/configyaml"
	"github.com/chenpingonline/Clash-Manager/backend/internal/runtimeenv"
)

// Apply deployment overrides before bootstrap or API requests can start Core.
// Persist them as user settings so applying subscriptions keeps the new ports.
func (h *helper) applyDockerStartupPorts() error {
	if !runtimeenv.Docker() {
		return nil
	}
	patch := map[string]any{}
	for _, item := range []struct{ env, key string }{
		{"APP_CONTROLLER_PORT", "external-controller"},
		{"APP_MIXED_PORT", "mixed-port"},
	} {
		value := strings.TrimSpace(os.Getenv(item.env))
		if value == "" {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("%s 必须为 1–65535 的整数", item.env)
		}
		if item.key == "external-controller" {
			patch[item.key] = fmt.Sprintf("127.0.0.1:%d", port)
		} else {
			patch[item.key] = port
		}
	}
	if len(patch) == 0 {
		return nil
	}
	settings, previous, err := h.readUserSettings()
	if err != nil {
		return err
	}
	if err = h.ensureManagedConfig(); err != nil {
		return err
	}
	raw, err := os.ReadFile(h.config.managedConfig)
	if err != nil {
		return err
	}
	content, err := configyaml.MergeOverrides(raw, patch)
	if err != nil {
		return fmt.Errorf("应用 Docker 启动端口失败: %w", err)
	}
	mergeUserSettings(settings, patch)
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicWrite(h.userSettingsPath(), body, 0600); err != nil {
		return err
	}
	if err = atomicWrite(h.config.managedConfig, content, 0640); err != nil {
		var restoreErr error
		if previous == nil {
			restoreErr = os.Remove(h.userSettingsPath())
		} else {
			restoreErr = atomicWrite(h.userSettingsPath(), previous, 0600)
		}
		return errors.Join(err, restoreErr)
	}
	return nil
}
