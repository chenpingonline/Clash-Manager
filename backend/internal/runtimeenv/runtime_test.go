package runtimeenv

import "testing"

func TestDeploymentPaths(t *testing.T) {
	t.Setenv("APP_CONFIG_DIR", "")
	t.Setenv("APP_STATE_DIR", "")
	t.Setenv("APP_DIR", "")
	t.Setenv("TRIM_PKGETC", "/native/etc")
	t.Setenv("TRIM_PKGVAR", "/native/var")
	t.Setenv("TRIM_APPDEST", "/native/app")
	if EtcDir() != "/native/etc" || VarDir() != "/native/var" || AppDir() != "/native/app" {
		t.Fatal("native paths changed")
	}
	t.Setenv("APP_CONFIG_DIR", "/data/config")
	t.Setenv("APP_STATE_DIR", "/data/state")
	t.Setenv("APP_DIR", "/opt/clash")
	if EtcDir() != "/data/config" || VarDir() != "/data/state" || AppDir() != "/opt/clash" {
		t.Fatal("deployment overrides ignored")
	}
}
