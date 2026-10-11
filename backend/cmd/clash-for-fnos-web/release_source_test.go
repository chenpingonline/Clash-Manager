package main

import (
	"testing"

	"github.com/chenpingonline/Clash-Manager/backend/internal/appupdate"
)

func TestNativeReleaseSourceMatchesInstaller(t *testing.T) {
	t.Setenv("CLASH_FOR_FNOS_RELEASE_REPO", "other/repository")
	t.Setenv("APP_PLATFORM", "fnos")
	if got := loadConfig().releaseRepo; got != appupdate.Repository {
		t.Fatalf("native checker %q differs from installer %q", got, appupdate.Repository)
	}
	t.Setenv("APP_PLATFORM", "docker")
	if got := loadConfig().releaseRepo; got != "other/repository" {
		t.Fatalf("standalone override lost: %q", got)
	}
}
