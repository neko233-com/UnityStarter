package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	cases := map[string]bool{
		"v" + appVersion: false,
		"v99.0.0":        true,
		"v0.0.1":         false,
	}
	for tag, want := range cases {
		if got := newerVersion(tag); got != want {
			t.Errorf("%s: %v, want %v (current %s)", tag, got, want, appVersion)
		}
	}
}

func TestCheckLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("no user agent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.com/release"}`))
	}))
	defer server.Close()

	previous := latestReleaseURL
	latestReleaseURL = server.URL
	defer func() { latestReleaseURL = previous }()

	release, err := checkLatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Tag != "v9.9.9" || release.URL != "https://example.com/release" {
		t.Errorf("release %+v", release)
	}
}
