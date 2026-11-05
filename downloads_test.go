package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The releases API's Windows installer is the one offered, and its date is
// read from the ISO string it sends.
func TestFetchUnityReleases(t *testing.T) {
	body := `{"results":[{"version":"6000.0.85f2","releaseDate":"2026-11-03T16:15:34.550Z","stream":"LTS",
		"downloads":[
			{"url":"https://download.unity3d.com/mac.dmg","type":"DMG","platform":"MAC_OS","architecture":"X86_64"},
			{"url":"https://download.unity3d.com/Windows64EditorInstaller/UnitySetup64-6000.0.85f2.exe","type":"EXE","platform":"WINDOWS","architecture":"X86_64","downloadSize":{"value":4096}}
		]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("accept %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	previous := unityReleasesURL
	unityReleasesURL = server.URL
	defer func() { unityReleasesURL = previous }()

	entries, err := fetchUnityReleases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("releases %v", entries)
	}
	got := entries[0]
	if got.Version != "6000.0.85f2" || got.Engine != kindUnity || !got.LTS || got.Size != 4096 {
		t.Errorf("release %+v", got)
	}
	if got.Date.Year() != 2026 || got.Date.Month() != 11 || got.Date.Day() != 3 {
		t.Errorf("date %v", got.Date)
	}
	if got.URL != "https://download.unity3d.com/Windows64EditorInstaller/UnitySetup64-6000.0.85f2.exe" {
		t.Errorf("url %s", got.URL)
	}
}
