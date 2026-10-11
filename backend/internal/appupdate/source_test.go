package appupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPinnedReleaseRepository(t *testing.T) {
	original := Repository
	t.Cleanup(func() { Repository = original })
	Repository = "chenpingonline/Clash-for-fnos-update-test"
	for _, repo := range []string{Repository, "chenpingonline/Clash-for-fnos"} {
		client := &http.Client{Transport: sourceTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://api.github.com/repos/"+Repository+"/releases/tags/v1.3.18" {
				t.Fatalf("wrong metadata source: %s", r.URL)
			}
			body := fmt.Sprintf(`{"tag_name":"v1.3.18","assets":[{"name":"Clash for fnos_1.3.18_x86_64.fpk","browser_download_url":"https://github.com/%s/releases/download/v1.3.18/package.fpk","size":42,"digest":"sha256:%s"}]}`, repo, strings.Repeat("a", 64))
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		_, err := resolve(context.Background(), client, "1.3.18", "amd64")
		if (err == nil) != (repo == Repository) {
			t.Fatalf("asset repo %s accepted incorrectly: %v", repo, err)
		}
	}
	Repository = "owner/repo/../../another"
	client := &http.Client{Transport: sourceTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("invalid repository reached network")
		return nil, nil
	})}
	if _, err := resolve(context.Background(), client, "1.3.18", "amd64"); err == nil {
		t.Fatal("invalid build source accepted")
	}
}

func TestRepositoryValidation(t *testing.T) {
	for _, value := range []string{"chenpingonline/Clash-for-fnos", "test-owner/test.repo_1"} {
		if !ValidRepository(value) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"", "https://github.com/o/r", "o/r?x=y", "o/..", "o/.", "o/r/extra", "o/r;cmd", "-o/r"} {
		if ValidRepository(value) {
			t.Fatal(value)
		}
	}
}
