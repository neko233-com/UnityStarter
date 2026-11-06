package main

import (
	"context"
	"sort"
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
	engine := engineForPage(a.page)
	entries := downloadsForEngine(a.downloads, engine)
	a.header(c, channelName(engine)+" · "+locales.PageDownloads(), locales.PageDownloadsHint(), func() {
		if engine == kindUnity {
			ui.Button(c, locales.DownloadsArchive()).OnClick(func() {
				_ = mygo.Shell.OpenExternal(urlUnityArchive)
			})
		}
		ui.Button(c, locales.ActionRefresh()).OnClick(func() { a.ensureDownloads(true) }).Disabled(a.downloadsBusy)
	})
	ui.Divider(c)

	if len(entries) == 0 {
		if a.downloadsBusy {
			ui.Row(c).Padding(26).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, locales.DownloadsLoading()).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		hint := a.downloadsErr
		if engine == kindTuanjie {
			hint = locales.TuanjieDownloadsEmptyHint()
		}
		a.emptyState(c, locales.DownloadsEmpty(), hint)
		return
	}

	installed := map[string]bool{}
	for _, e := range a.editors {
		installed[e.Kind+" "+e.Version] = true
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 26).Gap(20).MaxWidth(900).Children(func() {
			if engine == kindUnity && a.downloadsErr != "" {
				ui.Text(c, locales.DownloadsError(a.downloadsErr)).FontSize(11).TextColor(t.Danger)
			}
			title := locales.DownloadsUnity()
			if engine == kindTuanjie {
				title = locales.DownloadsTuanjie()
			}
			a.downloadGroup(c, title, engine, entries, installed)
		})
	})
}

func downloadsForEngine(entries []downloadEntry, engine string) []downloadEntry {
	var result []downloadEntry
	for _, e := range entries {
		if e.Engine == engine {
			result = append(result, e)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return compareVersions(result[i].Version, result[j].Version) > 0 })
	return result
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
					} else if e.Stream == "ALPHA" || e.Stream == "BETA" {
						ui.Badge(c, e.Stream)
					}
					if installed[engine+" "+e.Version] {
						ui.Badge(c, locales.DownloadsInstalled())
					}
				})
				ui.Text(c, a.formatTime(e.Date)).FontSize(11).TextColor(t.TextMuted).Width(96)
				ui.Text(c, humanSize(e.Size)).FontSize(11).TextColor(t.TextMuted).Width(80)
				ui.Spacer(c)
				a.downloadAction(c, e)
			})
		}
	})
}

// downloadAction is the button, bar or badge a release shows for its state.
func (a *app) downloadAction(c *ui.Context, e downloadEntry) {
	t := c.Theme()
	job := a.jobs[jobKey(e)]
	// In demo mode the flow is tried with a stand-in editor, not gigabytes.
	if a.prefs.DemoMode && (job == nil || job.Status == "installed") {
		if ui.PrimaryButton(c, locales.ActionSimulate()).Clicked() {
			if _, err := simulateEditorInstall(e.Engine, e.Version, a.installRoot()); err != nil {
				a.alert(err.Error())
				return
			}
			a.prefs.EditorRoots = dedupe(append(a.prefs.EditorRoots, a.installRoot()))
			a.save()
			a.refresh()
		}
		return
	}
	installed := false
	for _, editor := range a.editors {
		if editor.Version == e.Version && editor.Kind == e.Engine {
			installed = true
			break
		}
	}
	if installed && (job == nil || job.Status == "installed") {
		ui.Badge(c, locales.DownloadsInstalled())
		return
	}
	switch {
	case job == nil:
		if ui.PrimaryButton(c, locales.ActionDownload()).Clicked() {
			a.startDownload(e)
		}
	case job.Status == "downloading":
		fraction := 0.0
		if job.Total > 0 {
			fraction = float64(job.Done) / float64(job.Total)
		}
		ui.Progress(c, fraction).Width(150)
		ui.Textf(c, "%d%%", int(fraction*100)).FontSize(11).TextColor(t.TextMuted).Width(40)
		if ui.Button(c, locales.ActionCancel()).Clicked() && job.cancel != nil {
			job.cancel()
		}
	case job.Status == "downloaded":
		if ui.PrimaryButton(c, locales.ActionInstall()).Clicked() {
			a.installJob(job)
		}
	case job.Status == "installing":
		ui.Spinner(c)
		ui.Text(c, locales.DownloadsInstalling()).FontSize(11).TextColor(t.TextMuted)
	case job.Status == "failedDownload":
		ui.Text(c, locales.DownloadsFailed(job.Err)).FontSize(11).TextColor(t.Danger).MaxWidth(220).Clip()
		if ui.Button(c, locales.ActionDownload()).Clicked() {
			a.startDownload(e)
		}
	case job.Status == "failedInstall":
		ui.Text(c, locales.DownloadsInstallFailed(job.Err)).FontSize(11).TextColor(t.Danger).MaxWidth(220).Clip()
		if ui.PrimaryButton(c, locales.ActionInstall()).Clicked() {
			a.installJob(job)
		}
	default:
		ui.Badge(c, locales.DownloadsInstalled())
	}
}
