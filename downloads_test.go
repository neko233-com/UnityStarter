package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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

func TestFetchUnityReleaseVersionSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") != "2022.3.62f1" || r.URL.Query().Get("limit") != "25" {
			t.Errorf("query: %s", r.URL.RawQuery)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"version":"2022.3.62f1","downloads":[{"url":"https://download.unity3d.com/editor.exe","type":"EXE","platform":"WINDOWS","architecture":"X86_64"}]}]}`)
	}))
	defer server.Close()
	previous := unityReleasesURL
	unityReleasesURL = server.URL + "?limit=25"
	defer func() { unityReleasesURL = previous }()
	entries, err := fetchUnityReleasesForVersion(context.Background(), "2022.3.62f1")
	if err != nil || len(entries) != 1 || entries[0].Version != "2022.3.62f1" {
		t.Fatalf("version search: %+v %v", entries, err)
	}
}

func TestFindReleaseSearchesUnityArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") != "2022.3.62f1" {
			t.Errorf("query: %s", r.URL.RawQuery)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"version":"2022.3.62f1","downloads":[{"url":"https://download.unity3d.com/old.exe","type":"EXE","platform":"WINDOWS","architecture":"X86_64"}]}]}`)
	}))
	defer server.Close()
	previous := unityReleasesURL
	unityReleasesURL = server.URL + "?limit=25"
	defer func() { unityReleasesURL = previous }()
	entry, ok := findRelease("2022.3.62f1")
	if !ok || entry.Version != "2022.3.62f1" || entry.Engine != kindUnity {
		t.Fatalf("CLI archive lookup: %+v %v", entry, ok)
	}
}

func TestLiveLegacyUnityVersionSearch(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_CATALOG") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_CATALOG=1 to check official metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	entries, err := fetchUnityReleasesForVersion(ctx, "2022.3.62f1")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Version == "2022.3.62f1" {
			if !legacyUnityMD5(entry.Integrity) || editorDownloadExpectedSize(entry) != 0 {
				t.Fatalf("legacy official metadata not recognized: %+v", entry)
			}
			h, expected, err := integrityHasher(entry.Integrity)
			if err != nil || h == nil || len(expected) != 16 {
				t.Fatalf("official digest rejected: %v", err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, entry.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK || !strings.EqualFold(strings.Trim(resp.Header.Get("ETag"), `"`), hex.EncodeToString(expected)) {
				t.Fatalf("official file identity differs: status=%d etag=%q", resp.StatusCode, resp.Header.Get("ETag"))
			}
			t.Logf("official metadata size=%d, server size=%d", entry.Size, resp.ContentLength)
			return
		}
	}
	t.Fatalf("official version missing: %+v", entries)
}

func TestTuanjieCatalogWithoutHubAndOfflineFallback(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const catalog = `{"official":[{"version":"2022.3.62t17","downloadUrl":"https://download.tuanjie.cn/editor.exe","checksum":"0123456789abcdef0123456789abcdef","modules":[{"id":"webgl","name":"Web","downloadUrl":"https://download.tuanjie.cn/web.exe","visible":true}]}],"beta":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unity" {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(catalog))
	}))
	defer server.Close()
	oldUnity, oldTuanjie := unityReleasesURL, tuanjieReleasesURL
	unityReleasesURL, tuanjieReleasesURL = server.URL+"/unity", server.URL+"/tuanjie"
	defer func() { unityReleasesURL, tuanjieReleasesURL = oldUnity, oldTuanjie }()
	entries, errs := refreshReleaseCatalogs(context.Background())
	if len(entries) != 1 || entries[0].Engine != kindTuanjie || entries[0].Version != "2022.3.62t17" || errs[kindUnity] == "" || errs[kindTuanjie] != "" {
		t.Fatalf("independent catalogs: %+v errors=%v", entries, errs)
	}
	modules, err := moduleCatalog(context.Background(), editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Root: t.TempDir()})
	if err != nil || len(modules) != 1 || modules[0].ID != "webgl" {
		t.Fatalf("module cache: %+v %v", modules, err)
	}
	if err := os.Remove(tuanjieReleasesCachePath()); err != nil {
		t.Fatal(err)
	}
	modules, err = moduleCatalog(context.Background(), editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Root: t.TempDir()})
	if err != nil || len(modules) != 1 || modules[0].ID != "webgl" {
		t.Fatalf("module download without Hub: %+v %v", modules, err)
	}
	server.Close()
	entries, errs = refreshReleaseCatalogs(context.Background())
	if len(entries) != 1 || entries[0].Version != "2022.3.62t17" || errs[kindTuanjie] == "" {
		t.Fatalf("offline fallback: %+v errors=%v", entries, errs)
	}
	if data, err := os.ReadFile(tuanjieReleasesCachePath()); err != nil || string(data) != catalog {
		t.Fatalf("cache changed on error: %q %v", data, err)
	}
}

func TestReleaseCatalogFetchesStartIndependently(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tuanjieStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unity" {
			select {
			case <-tuanjieStarted:
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			case <-r.Context().Done():
			}
			return
		}
		close(tuanjieStarted)
		_, _ = w.Write([]byte(`{"official":[{"version":"2022.3.62t17","downloadUrl":"https://download.tuanjie.cn/editor.exe"}]}`))
	}))
	defer server.Close()
	oldUnity, oldTuanjie := unityReleasesURL, tuanjieReleasesURL
	unityReleasesURL, tuanjieReleasesURL = server.URL+"/unity", server.URL+"/tuanjie"
	defer func() { unityReleasesURL, tuanjieReleasesURL = oldUnity, oldTuanjie }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entries, errs := refreshReleaseCatalogs(ctx)
	if ctx.Err() != nil || len(entries) != 1 || errs[kindTuanjie] != "" || errs[kindUnity] == "" {
		t.Fatalf("catalogs did not run independently: %+v %v %v", entries, errs, ctx.Err())
	}
}

func TestTuanjieCatalogRejectsUntrustedEntries(t *testing.T) {
	for _, body := range []string{
		`{"official":[{"version":"../escape","downloadUrl":"https://download.tuanjie.cn/a.exe"}]}`,
		`{"official":[{"version":"1.0","downloadUrl":"https://evil.example/a.exe"}]}`,
		`{"official":[{"version":"1.0","downloadUrl":"https://download.tuanjie.cn/a.exe","checksum":"invalid"}]}`,
	} {
		if entries := parseTuanjieReleases([]byte(body)); len(entries) != 0 {
			t.Fatalf("untrusted entry accepted: %+v", entries)
		}
	}
}

func TestLiveTuanjieCatalog(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_CATALOG") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_CATALOG=1 for official network validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	data, err := fetchTuanjieCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entries := parseTuanjieReleases(data)
	for _, e := range entries {
		if e.Version == "2022.3.62t17" && strings.HasPrefix(e.URL, "https://download.tuanjie.cn/") && len(tuanjieModulesFromCatalog(data, e.Version)) > 0 {
			return
		}
	}
	t.Fatalf("expected release and modules missing from %d official entries", len(entries))
}

func TestFetchUnityAlpha(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"version":"7000.0.0a16","stream":"ALPHA","downloads":[{"url":"https://download.unity3d.com/UnitySetup64.exe","type":"EXE","platform":"WINDOWS","architecture":"X86_64","integrity":"md5-example"}]}]}`))
	}))
	defer server.Close()
	previous := unityReleasesURL
	unityReleasesURL = server.URL
	defer func() { unityReleasesURL = previous }()
	entries, err := fetchUnityReleases(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Stream != "ALPHA" || entries[0].Integrity != "md5-example" {
		t.Fatalf("alpha release: %+v, %v", entries, err)
	}
}
