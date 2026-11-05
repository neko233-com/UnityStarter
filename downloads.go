package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, unityReleasesURL, nil)
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
	if len(out) == 0 {
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

// tuanjieReleases reads the releases Tuanjie Hub keeps on this computer.
func tuanjieReleases() []downloadEntry {
	return tuanjieReleasesIn(hubConfigDirs())
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
		var file struct {
			Official []struct {
				Version      string `json:"version"`
				DownloadURL  string `json:"downloadUrl"`
				DownloadSize int64  `json:"downloadSize"`
				LTS          bool   `json:"lts"`
			} `json:"official"`
		}
		if json.Unmarshal(data, &file) != nil {
			continue
		}
		out := make([]downloadEntry, 0, len(file.Official))
		for _, r := range file.Official {
			if r.DownloadURL == "" {
				continue
			}
			out = append(out, downloadEntry{
				Engine:  kindTuanjie,
				Version: r.Version,
				LTS:     r.LTS,
				URL:     r.DownloadURL,
				Size:    r.DownloadSize,
			})
		}
		return out
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
