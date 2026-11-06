package main

import (
	"sort"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func (a *app) moduleProgressFraction() float64 {
	if a.moduleInstalling || a.moduleProgressTotal <= 0 {
		return -1
	}
	return min(1, max(0, float64(a.moduleProgressDone)/float64(a.moduleProgressTotal)))
}

func (a *app) moduleProgressView(c *ui.Context) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Progress(c, a.moduleProgressFraction()).Grow(1)
		if !a.moduleInstalling && a.moduleProgressTotal > 0 {
			ui.Text(c, humanSize(a.moduleProgressDone)+" / "+humanSize(a.moduleProgressTotal)).FontSize(11).TextColor(c.Theme().TextMuted)
		}
	})
}

func (a *app) activityJobs(activeOnly bool) []*downloadJob {
	var jobs []*downloadJob
	for _, job := range a.jobs {
		if job == nil || job.Status == "installed" {
			continue
		}
		if activeOnly && job.Status != "downloading" && job.Status != "installing" && job.Status != "clearing" {
			continue
		}
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		priority := func(status string) int {
			switch status {
			case "downloading", "installing", "clearing":
				return 0
			case "failedDownload", "failedInstall":
				return 1
			default:
				return 2
			}
		}
		if x, y := priority(jobs[i].Status), priority(jobs[j].Status); x != y {
			return x < y
		}
		if jobs[i].Entry.Engine != jobs[j].Entry.Engine {
			return jobs[i].Entry.Engine < jobs[j].Entry.Engine
		}
		return compareVersions(jobs[i].Entry.Version, jobs[j].Entry.Version) > 0
	})
	return jobs
}

func (a *app) editorActivityBanner(c *ui.Context) {
	if a.page == pageActivity {
		return
	}
	jobs := a.activityJobs(true)
	if len(jobs) == 0 {
		return
	}
	engine := a.activeEngine
	switch a.page {
	case pageProjects, pageFavorites, pageInstalls, pageDownloads, pageTuanjieProjects, pageTuanjieFavorites, pageTuanjieInstalls, pageTuanjieDownloads:
		engine = engineForPage(a.page)
	}
	var job *downloadJob
	for _, candidate := range jobs {
		if candidate.Entry.Engine == engine {
			job = candidate
			break
		}
	}
	ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		if job == nil {
			ui.Text(c, locales.ActivityOtherEngineTasks(len(jobs))).FontSize(12).Grow(1).Clip()
			ui.Button(c, locales.ActivityViewAll()).OnClick(func() { a.page = pageActivity })
			return
		}
		ui.Text(c, channelName(job.Entry.Engine)+" · "+job.Entry.Version).FontSize(12).Grow(1).Clip()
		if len(jobs) > 1 {
			ui.Text(c, locales.ActivityOtherJobs(len(jobs)-1)).FontSize(11).TextColor(c.Theme().TextMuted).Clip()
		}
		if job.Status == "downloading" && job.Total > 0 {
			ui.Progress(c, min(1, max(0, float64(job.Done)/float64(job.Total)))).Width(130)
		} else {
			ui.Progress(c, -1).Width(130)
		}
		ui.Button(c, locales.ActivityViewAll()).OnClick(func() { a.page = pageActivity })
	})
}

func (a *app) activityPage(c *ui.Context) {
	jobs := a.activityJobs(false)
	module := a.manageModules && (a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil || a.moduleStatus != "")
	a.header(c, locales.ActivityTitle(), locales.ActivityBackgroundHint(), nil)
	ui.Divider(c)
	if len(jobs) == 0 && !module && a.visualStudioTask == nil && a.templateJob == nil && !a.uninstallBusy && a.uninstallStatus == "" {
		a.emptyState(c, locales.ActivityEmpty(), "")
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24).Gap(16).Children(func() {
			a.uninstallActivityCard(c)
			a.templateActivityCard(c)
			if task := a.visualStudioTask; task != nil {
				c.Key("visual-studio")
				panel(c, func() {
					ui.Text(c, task.Module.Name).FontSize(14).Bold()
					status := locales.VisualStudioWaiting()
					if task.Status == "ready" {
						status = locales.VisualStudioVerified()
					} else if task.Status == "unverified" {
						status = locales.VisualStudioUnverified()
					}
					ui.Text(c, status).FontSize(12).TextColor(c.Theme().TextMuted).Clip().Tooltip(status)
					if task.Status == "waiting" {
						ui.Progress(c, -1).Width(180)
						ui.Button(c, locales.VisualStudioStopWatching()).OnClick(a.dismissVisualStudioTask).Tooltip(locales.VisualStudioStopWatchingHint())
					} else {
						ui.Button(c, locales.DialogOK()).OnClick(a.dismissVisualStudioTask)
					}
				})
			}
			if module {
				c.Key("module-task")
				panel(c, func() {
					ui.Text(c, displayVersion(a.manageEditor.Version, a.manageEditor.DisplayVersion)+" · "+a.moduleCurrentName).FontSize(14).Bold()
					status := a.moduleStatus
					if a.androidPlan != nil {
						status = locales.AndroidLicenseTitle()
					}
					ui.Text(c, status).FontSize(12).TextColor(c.Theme().TextMuted).Clip().Tooltip(status)
					if a.moduleBusy {
						a.moduleProgressView(c)
					}
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, locales.InstallModules()).OnClick(func() { a.openEditorManagement(a.manageEditor) })
						if a.moduleBusy && !a.moduleInstalling && a.moduleCancel != nil {
							ui.Button(c, locales.ActionCancel()).OnClick(func() {
								if a.moduleCancel != nil {
									a.moduleCancel()
								}
							})
						}
					})
				})
			}
			for _, job := range jobs {
				job := job
				c.Key(jobKey(job.Entry))
				panel(c, func() {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconBrand).Size(26, 26).TextColor(c.Theme().TextMuted)
						ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
							ui.Text(c, channelName(job.Entry.Engine)+" · "+job.Entry.Version).FontSize(14).Bold().SingleLine().Clip()
							if job.Total > 0 {
								ui.Text(c, humanSize(job.Done)+" / "+humanSize(job.Total)).FontSize(11).TextColor(c.Theme().TextMuted)
							}
						})
						badge := ui.Badge(c, downloadJobLabel(job))
						if job.Status == "failedDownload" || job.Status == "failedInstall" {
							badge.TextColor(c.Theme().Warning)
						}
					})
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() { a.downloadAction(c, job.Entry) })
				})
			}
		})
	})
}

func downloadJobLabel(job *downloadJob) string {
	switch job.Status {
	case "downloading":
		return locales.TaskDownloading()
	case "downloaded":
		return locales.TaskReady()
	case "installing":
		return locales.DownloadsInstalling()
	case "clearing":
		return locales.DownloadClearing()
	case "failedInstall", "failedDownload":
		return locales.TaskAttention()
	default:
		return locales.DownloadsInstalled()
	}
}
