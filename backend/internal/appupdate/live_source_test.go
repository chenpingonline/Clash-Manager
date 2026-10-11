package appupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Opt-in read-only check against published test assets. It never invokes App Center.
func TestLiveReleaseDownloadAndVerify(t *testing.T) {
	repo := os.Getenv("CLASH_UPDATE_LIVE_REPO")
	if repo == "" {
		t.Skip("optional published test release verification")
	}
	if repo != Repository {
		t.Fatal("explicit live repository must match compiled source")
	}
	target, arch := os.Getenv("CLASH_UPDATE_LIVE_VERSION"), os.Getenv("CLASH_UPDATE_LIVE_ARCH")
	ctx := context.Background()
	a, err := resolve(ctx, Client(), target, arch)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "update.fpk")
	if err = Download(ctx, Client(), a, file, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	if err = Verify(file, a.Digest, target, arch); err != nil {
		t.Fatal(err)
	}
	t.Logf("verified %s (%d bytes) from %s", a.Name, a.Size, Repository)
}
