package fnosapps

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDiscoveryScopesRulesToRealApplicationDirectory(t *testing.T) {
	root := t.TempDir()
	appsRoot := filepath.Join(root, "var", "apps")
	id := "music.demo"
	install := filepath.Join(root, "vol1", "@appcenter", id)
	if err := os.MkdirAll(install, 0700); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(appsRoot, id)
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "manifest"), []byte("appname = "+id+"\ndisplay_name = Music\nversion = 1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	install, err := filepath.EvalSymlinks(install)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(install, filepath.Join(app, "target")); err != nil {
		t.Fatal(err)
	}
	result, err := Discover(appsRoot)
	if err != nil || len(result) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if result[0].Name != "Music" || result[0].Reason != "" {
		t.Fatal(result)
	}
	pattern := strings.TrimSuffix(strings.TrimPrefix(result[0].Rule, "PROCESS-PATH-REGEX,"), ",DIRECT")
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(install, "bin", "fnmusic-go"), filepath.Join(app, "target", "bin", "fnmusic-go")} {
		if !re.MatchString(path) {
			t.Fatalf("missing %s", path)
		}
	}
	for _, path := range []string{install + "-other/bin/program", "/usr/bin/python3", strings.ReplaceAll(install, "music.demo", "musicXdemo") + "/bin/program"} {
		if re.MatchString(path) {
			t.Fatalf("overbroad match %s", path)
		}
	}
	// Installed but unresolved targets remain visible, with no generated rule.
	if err := os.Remove(filepath.Join(app, "target")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr", filepath.Join(app, "target")); err != nil {
		t.Fatal(err)
	}
	result, err = Discover(appsRoot)
	if err != nil || len(result) != 1 || result[0].Rule != "" || result[0].Reason == "" {
		t.Fatalf("unsafe target: %+v %v", result, err)
	}
	// Other files in /var/apps are not applications.
	if err := os.WriteFile(filepath.Join(app, "manifest"), []byte("appname = other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = Discover(appsRoot)
	if err != nil || len(result) != 0 {
		t.Fatalf("mismatched manifest: %+v %v", result, err)
	}
}

func TestDiscoveryDoesNotTreatMissingRootAsEmptyInstallation(t *testing.T) {
	if _, err := Discover(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected read error")
	}
}
