package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstalledAppsHelperRejectsStandaloneHosts(t *testing.T) {
	t.Setenv("APP_PLATFORM", "linux")
	response := httptest.NewRecorder()
	testHelper(t).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/system/installed-apps", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}
