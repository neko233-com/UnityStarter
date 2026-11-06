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
	sort.Slice(jobs, func(i, j int) bool { return jobKey(jobs[i].Entry) < jobKey(jobs[j].Entry) })
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
	a.header(c, locales.ActivityTitle(), "", func() {})
	ui.Divider(c)
	if len(jobs) == 0 && !module {
		a.emptyState(c, locales.ActivityEmpty(), "")
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 26).Gap(18).MaxWidth(900).Children(func() {
			if module {
				ui.Column(c).Gap(8).Children(func() {
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
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, channelName(job.Entry.Engine)+" · "+job.Entry.Version).FontSize(14).Bold()
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() { a.downloadAction(c, job.Entry) })
				})
			}
		})
	})
}
