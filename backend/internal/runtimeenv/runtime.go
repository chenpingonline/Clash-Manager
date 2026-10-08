// Package runtimeenv contains deployment differences shared by Web and Helper.
package runtimeenv

import (
	"os"
	"path/filepath"
	"strings"
)

func Value(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
func Platform() string { return Value("APP_PLATFORM", "fnos") }
func Docker() bool     { return Platform() == "docker" }
func EtcDir() string   { return Value("APP_CONFIG_DIR", Value("TRIM_PKGETC", "/tmp/clash-for-fnos-etc")) }
func VarDir() string   { return Value("APP_STATE_DIR", Value("TRIM_PKGVAR", "/tmp/clash-for-fnos-var")) }
func AppDir() string {
	return Value("APP_DIR", Value("TRIM_APPDEST", filepath.Clean(filepath.Join(filepath.Dir(os.Args[0]), "..", ".."))))
}
func AccessiblePaths() string {
	return Value("APP_IMPORT_PATHS", os.Getenv("TRIM_DATA_ACCESSIBLE_PATHS"))
}

// DisplayName preserves the native product name while branding standalone Docker deployments.
func DisplayName(platform string) string {
	if platform == "docker" {
		return "Clash Manager"
	}
	return "Clash for fnOS"
}
