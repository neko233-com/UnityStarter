package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"unitystarter/locales"
)

// latestReleaseURL asks GitHub for the newest release of this app.
var latestReleaseURL = "https://api.github.com/repos/neko233-com/UnityStarter/releases/latest"

type latestRelease struct {
	Tag string
	URL string
}

func checkLatestRelease(ctx context.Context) (latestRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return latestRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "UnityStarter/"+appVersion)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return latestRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return latestRelease{}, fmt.Errorf("releases: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return latestRelease{}, err
	}
	if body.TagName == "" {
		return latestRelease{}, fmt.Errorf("releases: no tag")
	}
	return latestRelease{Tag: body.TagName, URL: body.HTMLURL}, nil
}

// newerVersion reports whether a release tag names a version newer than this
// app, by the numbers of the version.
func newerVersion(tag string) bool {
	return compareVersions(strings.TrimPrefix(tag, "v"), appVersion) > 0
}

// checkUpdates asks GitHub for the newest release, and offers to open it.
func (a *app) checkUpdates() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		release, err := checkLatestRelease(ctx)
		if err != nil {
			logger.Printf("update: %v", err)
			a.alert(locales.UpdateFailed(err.Error()))
			return
		}
		logger.Printf("update: latest %s", release.Tag)
		if !newerVersion(release.Tag) {
			a.alert(locales.UpdateLatest())
			return
		}
		version := strings.TrimPrefix(release.Tag, "v")
		result, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Parent:        a.win,
			Message:       locales.UpdateAvailable(version),
			Buttons:       []string{locales.ActionCancel(), locales.ActionOpen()},
			DefaultButton: 1,
		})
		if result.Button == 1 {
			_ = mygo.Shell.OpenExternal(release.URL)
		}
	}()
}
