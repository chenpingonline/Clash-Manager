package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/chenpingonline/Clash-for-fnos/backend/internal/runtimeenv"
)

func gatewayPrefix() string {
	if value, ok := os.LookupEnv("GATEWAY_PREFIX"); ok {
		return strings.TrimRight(value, "/")
	}
	if runtimeenv.Docker() {
		return ""
	}
	return "/app/" + appName
}

func configureAuth(cfg *config) error {
	if cfg.platform != "fnos" && cfg.platform != "docker" {
		return errors.New("APP_PLATFORM 必须为 fnos 或 docker")
	}
	if file := os.Getenv("APP_AUTH_PASSWORD_FILE"); file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return errors.New("无法读取 APP_AUTH_PASSWORD_FILE")
		}
		cfg.authPassword = strings.TrimRight(string(body), "\r\n")
	}
	if (cfg.platform == "docker" || cfg.listenAddr != "") && len(cfg.authPassword) < 8 {
		return errors.New("HTTP/Docker 部署必须设置至少 8 字符的 APP_AUTH_PASSWORD 或 APP_AUTH_PASSWORD_FILE")
	}
	if cfg.platform == "docker" && cfg.listenAddr == "" {
		cfg.listenAddr = ":8080"
	}
	return nil
}

func listenWeb(cfg config) (net.Listener, error) {
	if cfg.listenAddr != "" {
		return net.Listen("tcp", cfg.listenAddr)
	}
	if err := removeStaleSocket(cfg.socketPath); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", cfg.socketPath)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(cfg.socketPath, 0o660); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func (g *gateway) runtimeCapabilities() map[string]any {
	platform := g.config.platform
	if platform == "" {
		platform = "fnos"
	}
	native := platform == "fnos"
	return map[string]any{"platform": platform, "version": version, "capabilities": map[string]bool{
		"appIcons": native, "appUpdates": native, "hostProxyEnvironment": native, "nativeFolderAuthorization": native, "externalCore": native,
	}}
}

func (g *gateway) handleUnsupportedDocker(w http.ResponseWriter, r *http.Request, path string) bool {
	if path == "/api/app/update-info" || path == "/api/app/check-update" {
		writeJSON(w, 200, map[string]any{"currentVersion": version, "sourceConfigured": false, "platform": "docker", "delivery": "image"})
		return true
	}
	if strings.HasPrefix(path, "/api/app/icon") || path == "/api/system/proxy-environment" {
		writeJSON(w, 409, map[string]string{"error": "此功能仅适用于 fnOS 原生应用"})
		return true
	}
	return false
}
