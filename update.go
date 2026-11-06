package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

// latestReleaseURL asks GitHub for the newest release of this app.
var latestReleaseURL = "https://api.github.com/repos/neko233-com/UnityStarter/releases/latest"

type latestRelease struct {
	Tag    string
	URL    string
	Assets []releaseAsset
}

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
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
		TagName string         `json:"tag_name"`
		HTMLURL string         `json:"html_url"`
		Assets  []releaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return latestRelease{}, err
	}
	if body.TagName == "" {
		return latestRelease{}, fmt.Errorf("releases: no tag")
	}
	return latestRelease{Tag: body.TagName, URL: body.HTMLURL, Assets: body.Assets}, nil
}

// newerVersion reports whether a release tag names a version newer than this
// app, by the numbers of the version.
func newerVersion(tag string) bool {
	return compareVersions(strings.TrimPrefix(tag, "v"), appVersion) > 0
}

// checkUpdates asks GitHub for the newest release, and offers to open it.
func (a *app) checkUpdates() {
	if isManagedInstall() {
		a.queueUpdateCheck(true)
		return
	}
	win := a.win
	base := a.updateCtx
	if base == nil {
		base = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, 20*time.Second)
		defer cancel()
		release, err := checkLatestRelease(ctx)
		if base.Err() != nil {
			return
		}
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
		if win != nil {
			win.Update(func() { a.availableVersion, a.availableURL, a.availableOpen = version, release.URL, true })
		}
	}()
}

func (a *app) availableUpdateDialog(c *ui.Context) {
	if !a.availableOpen {
		return
	}
	ui.Modal(c, &a.availableOpen, func() {
		ui.Column(c).Width(460).Gap(16).Children(func() {
			ui.Text(c, locales.AlertTitle()).FontSize(18).Bold()
			ui.Text(c, locales.UpdateAvailable(a.availableVersion)).FontSize(13)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.ActionCancel()).OnClick(func() { a.availableOpen = false })
				ui.PrimaryButton(c, locales.ActionOpen()).OnClick(func() { _ = mygo.Shell.OpenExternal(a.availableURL); a.availableOpen = false })
			})
		})
	})
}
