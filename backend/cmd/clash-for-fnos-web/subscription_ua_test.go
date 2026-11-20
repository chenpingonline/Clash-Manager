package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDynamicVergeUserAgent(t *testing.T) {
	tag := "v2.9.1"
	release := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"tag_name":%q}`, tag) }))
	defer release.Close()
	headers := make(chan string, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { headers <- r.UserAgent(); fmt.Fprint(w, "mode: rule") }))
	defer remote.Close()
	g := &gateway{vergeRelease: vergeReleaseCache{url: release.URL}}
	download := func(saved, want string) {
		t.Helper()
		_, _, _, _, _, err := g.downloadProfile(t.Context(), &profile{URL: remote.URL, UserAgent: saved})
		if err != nil {
			t.Fatal(err)
		}
		if got := <-headers; got != want {
			t.Fatalf("UA=%q, want %q", got, want)
		}
	}
	download("", "clash-verge/v2.9.1")
	tag = "v2.10.0"
	g.vergeRelease.nextCheck = time.Time{}
	download("clash-verge/v2.5.10", "clash-verge/v2.10.0")
	release.Close()
	g.vergeRelease.nextCheck = time.Time{}
	download("", "clash-verge/v2.10.0")
	download("custom-agent", "custom-agent")
}
