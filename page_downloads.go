package main

import (
	"context"
	"sort"
	"strings"
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
	a.downloadsPending = map[string]bool{kindUnity: true, kindTuanjie: true}
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		results := startReleaseCatalogFetch(ctx)
		for i := 0; i < 2; i++ {
			result := <-results
			win.Update(func() {
				a.applyReleaseCatalog(result)
				if len(a.downloadsPending) == 0 {
					a.downloadsBusy = false
					a.downloadsSince = time.Now()
				}
			})
		}
	}()
}

func (a *app) applyReleaseCatalog(result releaseCatalogResult) {
	remaining := make([]downloadEntry, 0, len(a.downloads)+len(result.entries))
	keptCurrent := false
	for _, entry := range a.downloads {
		if entry.Engine != result.engine || result.err != nil {
			remaining = append(remaining, entry)
			if entry.Engine == result.engine {
				keptCurrent = true
			}
		}
	}
	a.downloads = remaining
	if !keptCurrent {
		a.downloads = append(a.downloads, result.entries...)
	}
	if a.downloadsErrs == nil {
		a.downloadsErrs = map[string]string{}
	}
	if result.err != nil {
		a.downloadsErrs[result.engine] = result.err.Error()
	} else {
		delete(a.downloadsErrs, result.engine)
	}
	delete(a.downloadsPending, result.engine)
	a.downloadsReady = true
	logger.Printf("downloads: %s %d releases", result.engine, len(result.entries))
}

func (a *app) downloadsPage(c *ui.Context) {
	a.ensureDownloads(false)
	t := c.Theme()
	engine := engineForPage(a.page)
	entries := a.visibleDownloads(engine)
	matchCount := len(entries)
	active := a.activeDownloads(engine)
	activeKeys := make(map[string]bool, len(active))
	for _, e := range active {
		activeKeys[jobKey(e)] = true
	}
	remaining := entries[:0]
	for _, e := range entries {
		if !activeKeys[jobKey(e)] {
			remaining = append(remaining, e)
		}
	}
	entries = remaining
	a.header(c, channelName(engine)+" · "+locales.PageDownloads(), locales.PageDownloadsHint(), func() {
		if engine == kindUnity {
			ui.Button(c, locales.DownloadsArchive()).OnClick(func() {
				_ = mygo.Shell.OpenExternal(urlUnityArchive)
			})
		}
		ui.Button(c, locales.ActionRefresh()).OnClick(func() { a.ensureDownloads(true) }).Disabled(a.downloadsBusy)
	})
	ui.Row(c).Padding(0, 26, 14, 26).Gap(8).AlignItems(ui.Center).Children(func() {
		query := &a.downloadQueryUnity
		if engine == kindTuanjie {
			query = &a.downloadQueryTuanjie
		}
		ui.SearchField(c, query).Placeholder(locales.DownloadsSearchHint()).Label(locales.DownloadsSearchHint()).Width(250)
		if engine == kindUnity {
			ui.Button(c, locales.DownloadsSearchOfficial()).Disabled(a.unitySearchBusy || strings.TrimSpace(*query) == "").OnClick(func() { a.searchUnityReleases(strings.TrimSpace(a.downloadQueryUnity)) })
			if a.unitySearchBusy {
				ui.Spinner(c)
			}
		}
	})
	ui.Divider(c)

	if len(entries) == 0 && len(active) == 0 {
		if strings.TrimSpace(a.downloadQueryForEngine(engine)) != "" {
			hint := locales.DownloadsSearchEmptyHint()
			if engine == kindUnity && a.unitySearchQuery == strings.TrimSpace(a.downloadQueryUnity) && a.unitySearchErr != "" {
				hint = a.unitySearchErr
			}
			a.emptyState(c, locales.DownloadsSearchEmpty(), hint)
			return
		}
		if a.downloadsPending[engine] {
			ui.Row(c).Padding(26).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, locales.DownloadsLoading()).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		hint := a.downloadsErrs[engine]
		if hint == "" && engine == kindTuanjie {
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
			if engine == kindUnity && a.unitySearchQuery != "" && a.unitySearchQuery == strings.TrimSpace(a.downloadQueryUnity) && a.unitySearchErr == "" {
				ui.Text(c, locales.DownloadsSearchLimit()).FontSize(11).TextColor(t.TextMuted)
			}
			if err := a.downloadsErrs[engine]; err != "" {
				ui.Text(c, locales.DownloadsError(err)).FontSize(11).TextColor(t.Danger)
			}
			if engine == kindUnity && a.unitySearchQuery == strings.TrimSpace(a.downloadQueryUnity) && a.unitySearchErr != "" {
				ui.Text(c, locales.DownloadsError(a.unitySearchErr)).FontSize(11).TextColor(t.Danger)
			}
			if len(active) > 0 {
				a.downloadGroup(c, locales.DownloadsActive(), engine, active, installed)
			}
			if matchCount == 0 && strings.TrimSpace(a.downloadQueryForEngine(engine)) != "" {
				ui.Text(c, locales.DownloadsSearchEmpty()).FontSize(12).TextColor(t.TextMuted)
			}
			title := locales.DownloadsUnity()
			if engine == kindTuanjie {
				title = locales.DownloadsTuanjie()
			}
			a.downloadGroup(c, title, engine, entries, installed)
		})
	})
}

func (a *app) activeDownloads(engine string) []downloadEntry {
	var active []downloadEntry
	for _, job := range a.jobs {
		if job != nil && job.Entry.Engine == engine && job.Status != "installed" {
			active = append(active, job.Entry)
		}
	}
	return downloadsForEngine(active, engine)
}

func (a *app) downloadQueryForEngine(engine string) string {
	if engine == kindTuanjie {
		return a.downloadQueryTuanjie
	}
	return a.downloadQueryUnity
}

func (a *app) visibleDownloads(engine string) []downloadEntry {
	query := strings.TrimSpace(a.downloadQueryForEngine(engine))
	entries := a.downloads
	if engine == kindUnity && query != "" && query == a.unitySearchQuery && a.unitySearchErr == "" {
		entries = a.unitySearchEntries
	}
	filtered := downloadsForEngine(entries, engine)
	if query == "" {
		return filtered
	}
	q := strings.ToLower(query)
	result := filtered[:0]
	for _, entry := range filtered {
		if strings.Contains(strings.ToLower(entry.Version), q) {
			result = append(result, entry)
		}
	}
	return result
}

func (a *app) searchUnityReleases(query string) {
	if a.win == nil || a.unitySearchBusy || query == "" {
		return
	}
	a.unitySearchBusy = true
	win := a.win
	parent := a.updateCtx
	if parent == nil {
		parent = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, 25*time.Second)
		defer cancel()
		entries, err := fetchUnityReleasesForVersion(ctx, query)
		win.Update(func() { a.applyUnityReleaseSearch(query, entries, err) })
	}()
}

func (a *app) applyUnityReleaseSearch(query string, entries []downloadEntry, err error) {
	a.unitySearchBusy = false
	if strings.TrimSpace(a.downloadQueryUnity) != query {
		return
	}
	a.unitySearchQuery = query
	a.unitySearchErr = ""
	if err != nil {
		a.unitySearchErr = err.Error()
		return
	}
	a.unitySearchEntries = entries
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
			job := a.jobs[jobKey(e)]
			ui.Column(c).Padding(7, 0).Gap(5).Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
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
				})
				if !installed[engine+" "+e.Version] || (job != nil && job.Status != "installed") {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() { a.downloadAction(c, e) })
				}
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
		if job.Total > 0 {
			ui.Textf(c, "%d%%", int(fraction*100)).FontSize(11).TextColor(t.TextMuted).Width(40)
		} else {
			ui.Text(c, humanSize(job.Done)).FontSize(11).TextColor(t.TextMuted)
		}
		if job.Speed > 0 {
			ui.Text(c, humanSize(job.Speed)+"/s").FontSize(11).TextColor(t.TextMuted).NoWrap()
		}
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
		ui.Text(c, locales.DownloadsFailed(job.Err)).FontSize(11).TextColor(t.Danger).NoWrap().Grow(1).Clip().Tooltip(job.Err)
		if ui.Button(c, locales.ActionRetryDownload()).Clicked() {
			a.startDownload(e)
		}
		ui.Button(c, locales.ActionDiscardDownload()).OnClick(func() {
			a.discardDownload(e, job)
		})
	case job.Status == "clearing":
		ui.Spinner(c)
		ui.Text(c, locales.DownloadClearing()).FontSize(11).TextColor(t.TextMuted)
	case job.Status == "failedInstall":
		ui.Text(c, locales.DownloadsInstallFailed(job.Err)).FontSize(11).TextColor(t.Danger).MaxWidth(220).Clip()
		if ui.PrimaryButton(c, locales.ActionInstall()).Clicked() {
			a.installJob(job)
		}
	default:
		ui.Badge(c, locales.DownloadsInstalled())
	}
}
