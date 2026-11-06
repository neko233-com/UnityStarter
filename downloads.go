package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// downloadEntry is an editor installer of either engine.
type downloadEntry struct {
	Engine    string    `json:"engine"`
	Version   string    `json:"version"`
	Date      time.Time `json:"date"`
	LTS       bool      `json:"lts"`
	Stream    string    `json:"stream,omitempty"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	Integrity string    `json:"integrity,omitempty"`
}

// unityReleasesURL lists the international releases from Unity's own API,
// whose installers come from download.unity3d.com.
var unityReleasesURL = "https://services.api.unity.com/unity/editor/release/v1/releases?limit=25"

type unityRelease struct {
	Version     string          `json:"version"`
	ReleaseDate string          `json:"releaseDate"`
	Stream      string          `json:"stream"`
	Downloads   []unityDownload `json:"downloads"`
}

type unityDownload struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	Integrity    string `json:"integrity"`
	DownloadSize struct {
		Value int64 `json:"value"`
	} `json:"downloadSize"`
}

// fetchUnityReleases reads the international releases, whose Windows
// installer is the one the release lists.
func fetchUnityReleases(ctx context.Context) ([]downloadEntry, error) {
	return fetchUnityReleasesForVersion(ctx, "")
}

func fetchUnityReleasesForVersion(ctx context.Context, version string) ([]downloadEntry, error) {
	endpoint, err := url.Parse(unityReleasesURL)
	if err != nil {
		return nil, err
	}
	if version != "" {
		query := endpoint.Query()
		query.Set("version", version)
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unity releases: %s", resp.Status)
	}
	var body struct {
		Results []unityRelease `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var out []downloadEntry
	for _, r := range body.Results {
		entry := downloadEntry{Engine: kindUnity, Version: r.Version, LTS: r.Stream == "LTS", Stream: r.Stream}
		if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(r.ReleaseDate)); err == nil {
			entry.Date = t
		}
		for _, d := range r.Downloads {
			if d.Platform == "WINDOWS" && d.Architecture == "X86_64" && d.Type == "EXE" && strings.HasSuffix(strings.ToLower(d.URL), ".exe") {
				entry.URL = d.URL
				entry.Size = d.DownloadSize.Value
				entry.Integrity = d.Integrity
				break
			}
		}
		if entry.URL != "" {
			out = append(out, entry)
		}
	}
	if len(out) == 0 && version == "" {
		return nil, fmt.Errorf("unity releases: none")
	}
	return out, nil
}

func unityReleasesCachePath() string {
	return filepath.Join(userDataDir(), "releases-unity.json")
}

func readCachedUnityReleases() []downloadEntry {
	var out []downloadEntry
	if data, err := os.ReadFile(unityReleasesCachePath()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func writeCachedUnityReleases(entries []downloadEntry) {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	_ = writeFileAtomic(unityReleasesCachePath(), data)
}

// tuanjieReleases reads our own official catalog cache, falling back to the
// installed Hub's cache when this app has not fetched the catalog yet.
func tuanjieReleases() []downloadEntry {
	if data, err := os.ReadFile(tuanjieReleasesCachePath()); err == nil {
		if entries := parseTuanjieReleases(data); len(entries) > 0 {
			return entries
		}
	}
	return tuanjieReleasesIn(hubConfigDirs())
}

var tuanjieReleasesURL = "https://public-cdn.tuanjie.cn/hub/prod/releases-win32.json"

type tuanjieRelease struct {
	Version      string         `json:"version"`
	DownloadURL  string         `json:"downloadUrl"`
	DownloadSize int64          `json:"downloadSize"`
	LTS          bool           `json:"lts"`
	Checksum     string         `json:"checksum"`
	Modules      []editorModule `json:"modules"`
}

type tuanjieCatalog struct {
	Official []tuanjieRelease `json:"official"`
	Beta     []tuanjieRelease `json:"beta"`
}

func tuanjieReleasesCachePath() string {
	return filepath.Join(userDataDir(), "releases-tuanjie.json")
}

func fetchTuanjieCatalog(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tuanjieReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("Tuanjie catalog unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tuanjie catalog: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 || len(parseTuanjieReleases(data)) == 0 {
		return nil, fmt.Errorf("invalid Tuanjie catalog")
	}
	return data, nil
}

func parseTuanjieReleases(data []byte) []downloadEntry {
	var file tuanjieCatalog
	if json.Unmarshal(data, &file) != nil {
		return nil
	}
	var out []downloadEntry
	for index, r := range append(file.Official, file.Beta...) {
		u, err := url.Parse(r.DownloadURL)
		if err != nil || u.Scheme != "https" || u.Host != "download.tuanjie.cn" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(strings.ToLower(u.Path), ".exe") || !moduleIDPattern.MatchString(r.Version) {
			continue
		}
		integrity := ""
		if r.Checksum != "" {
			digest, err := hex.DecodeString(r.Checksum)
			if err != nil || len(digest) != 16 {
				continue
			}
			integrity = "md5-" + base64.StdEncoding.EncodeToString(digest)
		}
		stream := ""
		if index >= len(file.Official) {
			stream = "BETA"
		}
		out = append(out, downloadEntry{Engine: kindTuanjie, Version: r.Version, LTS: r.LTS, URL: r.DownloadURL, Size: r.DownloadSize, Stream: stream, Integrity: integrity})
	}
	return out
}

// Both catalogs are independent. A timeout or an error in either service
// leaves the other engine and any last known catalog usable.
type releaseCatalogResult struct {
	engine  string
	entries []downloadEntry
	err     error
}

func startReleaseCatalogFetch(ctx context.Context) <-chan releaseCatalogResult {
	results := make(chan releaseCatalogResult, 2)
	go func() {
		entries, err := fetchUnityReleases(ctx)
		if err != nil {
			entries = readCachedUnityReleases()
		} else {
			writeCachedUnityReleases(entries)
		}
		results <- releaseCatalogResult{kindUnity, entries, err}
	}()
	go func() {
		data, err := fetchTuanjieCatalog(ctx)
		var entries []downloadEntry
		if err != nil {
			entries = tuanjieReleases()
		} else {
			entries = parseTuanjieReleases(data)
			if cacheErr := writeFileAtomic(tuanjieReleasesCachePath(), data); cacheErr != nil {
				logger.Printf("Tuanjie catalog cache: %v", cacheErr)
			}
		}
		results <- releaseCatalogResult{kindTuanjie, entries, err}
	}()
	return results
}

func refreshReleaseCatalogs(ctx context.Context) ([]downloadEntry, map[string]string) {
	results := startReleaseCatalogFetch(ctx)
	var unity, tuanjie []downloadEntry
	errors := map[string]string{}
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			errors[r.engine] = r.err.Error()
		}
		if r.engine == kindUnity {
			unity = r.entries
		} else {
			tuanjie = r.entries
		}
	}
	return append(unity, tuanjie...), errors
}

func tuanjieReleasesIn(dirs []string) []downloadEntry {
	for _, dir := range dirs {
		if !strings.Contains(strings.ToLower(filepath.Base(dir)), "tuanjie") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "releases.json"))
		if err != nil {
			continue
		}
		if out := parseTuanjieReleases(data); len(out) > 0 {
			return out
		}
	}
	return nil
}

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
