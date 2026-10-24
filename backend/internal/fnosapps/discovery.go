// Package fnosapps discovers native fnOS installations without executing apps
// or accepting paths from HTTP clients.
package fnosapps

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chenpingonline/Clash-Manager/backend/internal/appupdate"
)

type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Reason string `json:"reason,omitempty"`
}

var appID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Discover reads bounded manifest files. Root is supplied only by the host
// adapter (or tests), never by a request parameter.
func Discover(root string) ([]App, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取飞牛已安装应用失败: %w", err)
	}
	if len(entries) > 2048 {
		return nil, fmt.Errorf("飞牛应用数量超过读取限制")
	}
	apps := []App{}
	for _, entry := range entries {
		id := entry.Name()
		if !appID.MatchString(id) {
			continue
		}
		file, err := os.Open(filepath.Join(root, id, "manifest"))
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(file, 65537))
		file.Close()
		if err != nil || len(body) > 65536 {
			continue
		}
		manifest, err := appupdate.Manifest(string(body))
		if err != nil || manifest["appname"] != id {
			continue
		}
		name := strings.TrimSpace(manifest["display_name"])
		if name == "" {
			name = id
		}
		app := App{ID: id, Name: name}
		target := filepath.Join(root, id, "target")
		resolved, err := filepath.EvalSymlinks(target)
		info, statErr := os.Stat(target)
		if err != nil || statErr != nil || !info.IsDir() || !safeTarget(resolved, id) {
			app.Reason = "无法确认应用专属安装目录"
		} else if strings.ContainsAny(target+resolved, ",\r\n\x00") {
			app.Reason = "应用路径包含不支持的字符"
		} else {
			// Include the fnOS alias as well as the real path used by /proc/PID/exe.
			app.Rule = "PROCESS-PATH-REGEX,^(" + regexp.QuoteMeta(target) + "|" + regexp.QuoteMeta(resolved) + ")/,DIRECT"
		}
		apps = append(apps, app)
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Name == apps[j].Name {
			return apps[i].ID < apps[j].ID
		}
		return apps[i].Name < apps[j].Name
	})
	return apps, nil
}

func safeTarget(path, id string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	// A package target must be scoped to its own app, never a shared system root.
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	if len(parts) < 4 {
		return false
	}
	for _, name := range []string{"bin", "sbin", "usr", "etc", "proc", "sys", "dev", "lib", "lib64"} {
		if parts[1] == name {
			return false
		}
	}
	for _, part := range parts[1:] {
		if part == id {
			return true
		}
	}
	return false
}
