package main

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

const urlUnityArchive = "https://unity.com/releases/editor/archive"

// ensureDownloads reads the releases: the cached list at once, then Unity's
// own API, whose installers come from download.unity3d.com.
func (a *app) ensureDownloads(force bool) {
	if a.downloadsBusy {
		return
	}
	if !force && a.downloadsReady && time.Since(a.downloadsSince) < 10*time.Minute {
		return
	}
	if !a.downloadsReady {
		unities := readCachedUnityReleases()
		a.downloads = append(unities, tuanjieReleases()...)
		a.downloadsReady = len(a.downloads) > 0
	}
	// A view without a window, as in tests, reads only what is on disk.
	if a.win == nil {
		return
	}
	a.downloadsBusy = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		entries, err := fetchUnityReleases(ctx)
		a.win.Update(func() {
			a.downloadsBusy = false
			if err != nil {
				logger.Printf("downloads: %v", err)
				a.downloadsErr = err.Error()
				if len(a.downloads) == 0 {
					a.downloads = tuanjieReleases()
				}
				return
			}
			a.downloadsErr = ""
			writeCachedUnityReleases(entries)
			a.downloads = append(entries, tuanjieReleases()...)
			a.downloadsSince = time.Now()
			a.downloadsReady = true
			logger.Printf("downloads: %d releases", len(a.downloads))
		})
	}()
}

func (a *app) downloadsPage(c *ui.Context) {
	a.ensureDownloads(false)
	t := c.Theme()
	a.header(c, locales.PageDownloads(), locales.PageDownloadsHint(), func() {
		ui.Button(c, locales.DownloadsArchive()).OnClick(func() {
			_ = mygo.Shell.OpenExternal(urlUnityArchive)
		})
		ui.Button(c, locales.ActionRefresh()).OnClick(func() { a.ensureDownloads(true) }).Disabled(a.downloadsBusy)
	})
	ui.Divider(c)

	if len(a.downloads) == 0 {
		if a.downloadsBusy {
			ui.Row(c).Padding(26).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, locales.DownloadsLoading()).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		a.emptyState(c, locales.DownloadsEmpty(), a.downloadsErr)
		return
	}

	installed := map[string]bool{}
	for _, e := range a.editors {
		installed[e.Kind+" "+e.Version] = true
	}
	unity, tuanjie := splitDownloads(a.downloads)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 26).Gap(20).MaxWidth(900).Children(func() {
			if a.downloadsErr != "" {
				ui.Text(c, locales.DownloadsError(a.downloadsErr)).FontSize(11).TextColor(t.Danger)
			}
			a.downloadGroup(c, locales.DownloadsUnity(), kindUnity, unity, installed)
			a.downloadGroup(c, locales.DownloadsTuanjie(), kindTuanjie, tuanjie, installed)
		})
	})
}

func splitDownloads(entries []downloadEntry) (unity, tuanjie []downloadEntry) {
	for _, e := range entries {
		if e.Engine == kindTuanjie {
			tuanjie = append(tuanjie, e)
		} else {
			unity = append(unity, e)
		}
	}
	return unity, tuanjie
}

func (a *app) downloadGroup(c *ui.Context, title, engine string, entries []downloadEntry, installed map[string]bool) {
	if len(entries) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(15).Bold()
			ui.Badge(c, channelName(engine))
		})
		for _, e := range entries {
			e := e
			ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(7, 0).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Width(260).Children(func() {
					ui.Text(c, e.Version).FontSize(14).Bold()
					if e.LTS {
						ui.Badge(c, "LTS")
					}
					if installed[engine+" "+e.Version] {
						ui.Badge(c, locales.DownloadsInstalled())
					}
				})
				ui.Text(c, a.formatTime(e.Date)).FontSize(11).TextColor(t.TextMuted).Width(96)
				ui.Text(c, humanSize(e.Size)).FontSize(11).TextColor(t.TextMuted).Width(80)
				ui.Spacer(c)
				if ui.PrimaryButton(c, locales.ActionDownload()).Clicked() {
					_ = mygo.Shell.OpenExternal(e.URL)
				}
			})
		}
	})
}
