package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

type templateDownloadJob struct {
	Template    projectTemplate
	Editor      editorInstall
	Done, Total int64
	Busy        bool
	Err         string
	cancel      context.CancelFunc
}

func (a *app) loadProjectTemplates(e editorInstall, force bool) {
	key := templateEditorKey(e)
	if a.newTemplateSource == key && !force {
		return
	}
	if a.newTemplatesCancel != nil {
		a.newTemplatesCancel()
	}
	a.newTemplatesGeneration++
	generation := a.newTemplatesGeneration
	if a.newTemplateSource != key {
		a.newTemplates = nil
		a.newTemplateID = ""
		a.newTemplateChosen = false
		a.newTemplateTerms = false
	}
	a.newTemplateSource, a.newTemplatesErr = key, ""
	if a.win == nil {
		return
	}
	a.newTemplatesBusy = true
	win := a.win
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	a.newTemplatesCancel = cancel
	go func() {
		defer cancel()
		local := localProjectTemplates(e)
		cached := cachedProjectTemplates(e)
		initial := mergeProjectTemplates(e, local, cached)
		win.Update(func() {
			if generation == a.newTemplatesGeneration {
				a.newTemplates = initial
				a.chooseDefaultProjectTemplate()
			}
		})
		remote, err := fetchProjectTemplates(ctx, e)
		if ctx.Err() != nil && len(remote) == 0 {
			err = ctx.Err()
		}
		if err == nil {
			if data, encodeErr := json.Marshal(remote); encodeErr == nil {
				_ = writeFileAtomic(templateCatalogPath(e), data)
			}
		} else {
			remote = cached
		}
		items := mergeProjectTemplates(e, local, remote)
		win.Update(func() {
			if generation != a.newTemplatesGeneration {
				return
			}
			a.newTemplatesBusy = false
			a.newTemplates = items
			if err != nil {
				a.newTemplatesErr = locales.TemplateCatalogFailed(err.Error())
			}
			a.chooseDefaultProjectTemplate()
		})
	}()
}

func (a *app) chooseDefaultProjectTemplate() {
	if a.newTemplateChosen {
		return
	}
	for _, t := range a.newTemplates {
		if strings.HasSuffix(t.ID, ".urp-blank") || strings.HasSuffix(t.ID, ".universal") {
			a.newTemplateID = t.key()
			return
		}
	}
	if len(a.newTemplates) > 0 {
		a.newTemplateID = a.newTemplates[0].key()
	}
}

func (a *app) selectedProjectTemplate() (projectTemplate, bool) {
	for _, t := range a.newTemplates {
		if t.key() == a.newTemplateID {
			return t, true
		}
	}
	return projectTemplate{}, false
}

func (a *app) filteredProjectTemplates() []projectTemplate {
	var result []projectTemplate
	q := strings.ToLower(strings.TrimSpace(a.newTemplateQuery))
	for _, t := range a.newTemplates {
		if a.newTemplateCategory == "local" && t.Archive == "" {
			continue
		}
		if a.newTemplateCategory != "" && a.newTemplateCategory != "local" && t.Category != a.newTemplateCategory {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.Name+" "+t.ID+" "+t.Description+" "+t.Pipeline), q) {
			continue
		}
		result = append(result, t)
	}
	return result
}

func (a *app) templateWorkBusy() bool {
	return a.newCreating || a.templateJob != nil && a.templateJob.Busy
}

func (a *app) startTemplateDownload(e editorInstall, t projectTemplate) {
	if a.win == nil || a.templateWorkBusy() {
		return
	}
	if err := validateRemoteTemplate(e.Kind, &t); err != nil {
		a.newError = err.Error()
		return
	}
	if t.TermsURL != "" && !a.newTemplateTerms {
		a.newError = locales.TemplateTermsRequired()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	if a.updateCtx != nil {
		ctx, cancel = context.WithCancel(a.updateCtx)
	}
	job := &templateDownloadJob{Template: t, Editor: e, Busy: true, cancel: cancel}
	a.templateJob = job
	a.newError = ""
	file := templateCachePath(e.Kind, t)
	win := a.win
	go func() {
		defer cancel()
		err := downloadFileVerified(ctx, t.URL, file, t.Integrity, func(done, total int64) {
			win.Update(func() {
				if a.templateJob == job {
					job.Done, job.Total = done, total
				}
			})
		})
		if err == nil {
			t.Archive = file
			err = verifyProjectTemplate(t, e.Kind)
			if err != nil {
				_ = os.Remove(file)
			}
		}
		win.Update(func() {
			if a.templateJob != job {
				return
			}
			job.Busy = false
			if err != nil {
				job.Err = err.Error()
				if a.newTemplateSource == templateEditorKey(e) {
					a.newError = locales.TemplateDownloadFailed(err.Error())
				}
				return
			}
			job.Template = t
			if a.newTemplateSource == templateEditorKey(e) {
				for i := range a.newTemplates {
					if a.newTemplates[i].key() == t.key() {
						a.newTemplates[i].Archive = file
					}
				}
			}
		})
	}()
}

func (a *app) newProjectDialog(c *ui.Context) {
	if !a.newOpen {
		return
	}
	t := c.Theme()
	w, h := c.Size()
	width := min(float32(1080), w-96)
	bodyHeight := min(float32(540), max(float32(220), h-280))
	ui.Modal(c, &a.newOpen, func() {
		ui.Column(c).Width(width).Gap(14).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconProjects).Size(24, 24).TextColor(t.TextMuted)
				ui.Text(c, locales.NewProjectTitle()).FontSize(22).Bold().Grow(1)
				ui.Button(c, "×").Width(32).Label(locales.TemplateClose()).Tooltip(locales.TemplateClose()).Disabled(a.newCreating).OnClick(func() { a.newOpen = false })
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.NewProjectEditor()).FontSize(12).TextColor(t.TextMuted)
				labels, exes := a.editorLabels()
				if len(labels) > 0 {
					chosen := labels[indexOf(exes, a.newEditor)]
					ui.Select(c, &chosen, labels).Label(locales.NewProjectEditor()).Width(300).Disabled(a.templateWorkBusy())
					a.newEditor = exes[indexOf(labels, chosen)]
				}
				ui.Spacer(c)
				if e, ok := a.editorInstall(a.newEditor); ok {
					if a.newTemplateSource != templateEditorKey(e) {
						a.loadProjectTemplates(e, false)
					}
					ui.Button(c, locales.TemplateRefresh()).Disabled(a.newTemplatesBusy || a.templateWorkBusy()).OnClick(func() { a.loadProjectTemplates(e, true) })
				}
			})
			ui.Divider(c)
			ui.Row(c).Height(bodyHeight).Gap(20).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Gap(8).Children(func() {
					ui.SearchField(c, &a.newTemplateQuery).Placeholder(locales.TemplateSearch()).Label(locales.TemplateSearch())
					ui.Row(c).Gap(0).Wrap().Children(func() {
						for _, tab := range []struct{ id, label string }{{"", locales.InstallAll()}, {"CORE", locales.TemplateCore()}, {"SAMPLE", locales.TemplateSample()}, {"LEARNING", locales.TemplateLearning()}, {"local", locales.TemplateLocal()}} {
							tab := tab
							pageTab(c, tab.label, a.newTemplateCategory == tab.id, func() { a.newTemplateCategory = tab.id })
						}
					})
					if a.newTemplatesBusy {
						ui.Row(c).Gap(8).Children(func() { ui.Spinner(c); ui.Text(c, locales.TemplateLoading()).FontSize(12).TextColor(t.TextMuted) })
					}
					if a.newTemplatesErr != "" {
						ui.Text(c, a.newTemplatesErr).FontSize(11).TextColor(t.Warning).SingleLine().Clip().Tooltip(a.newTemplatesErr)
					}
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
						items := a.filteredProjectTemplates()
						if len(items) == 0 && !a.newTemplatesBusy {
							ui.Text(c, locales.TemplateEmpty()).Padding(18).FontSize(12).TextColor(t.TextMuted)
						}
						columns := 2
						if width < 900 {
							columns = 1
						}
						ui.Grid(c).Columns(columns).Gap(10).Children(func() {
							for _, item := range items {
								a.projectTemplateTile(c, item)
							}
						})
					})
				})
				ui.Column(c).Width(300).MinHeight(0).Gap(10).Children(func() {
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { ui.Column(c).Gap(14).Children(func() { a.projectTemplateDetails(c) }) })
				})
			})
			ui.Divider(c)
			if a.newError != "" {
				ui.Text(c, a.newError).FontSize(12).TextColor(t.Danger).SingleLine().Clip().Tooltip(a.newError)
			}
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				if job := a.templateJob; job != nil && job.Busy {
					fraction := -1.0
					if job.Total > 0 {
						fraction = min(1, float64(job.Done)/float64(job.Total))
					}
					ui.Progress(c, fraction).Grow(1).MinWidth(40)
					ui.Text(c, humanSize(job.Done)+" / "+humanSize(job.Total)).FontSize(11).TextColor(t.TextMuted)
					ui.Button(c, locales.ActionCancel()).OnClick(job.cancel)
					ui.Button(c, locales.ActionBackground()).OnClick(func() { a.newOpen = false; a.page = pageActivity })
				} else {
					ui.Text(c, locales.TemplateCompatibilityHint()).FontSize(11).TextColor(t.TextMuted).Grow(1)
				}
				template, selected := a.selectedProjectTemplate()
				if selected && template.Archive == "" {
					ui.PrimaryButton(c, locales.TemplateDownload()).Disabled(a.templateWorkBusy() || template.URL == "" || template.TermsURL != "" && !a.newTemplateTerms).OnClick(func() {
						if e, ok := a.editorInstall(a.newEditor); ok {
							a.startTemplateDownload(e, template)
						}
					})
				} else {
					label := locales.ActionCreate()
					if a.newCreating {
						label = locales.TemplateCreating()
					}
					ui.PrimaryButton(c, label).Disabled(a.templateWorkBusy() || !selected || strings.TrimSpace(a.newName) == "" || a.newDir == "" || template.TermsURL != "" && !a.newTemplateTerms).OnClick(a.createProject)
				}
			})
		})
	})
}

func (a *app) projectTemplateTile(c *ui.Context, item projectTemplate) {
	t := c.Theme()
	c.Key(item.key())
	selected := a.newTemplateID == item.key()
	button := ui.Button(c, "").Label(item.Name).Padding(14).MinHeight(98).Border(1, t.Border).Background(t.Background).Disabled(a.newCreating).OnClick(func() {
		a.newTemplateID = item.key()
		a.newTemplateChosen = true
		a.newTemplateTerms = false
		a.newError = ""
	})
	if selected {
		button.Border(2, t.Accent).Background(t.Accent.Alpha(0.12))
	} else if button.Hovered() {
		button.Background(t.SurfaceHover)
	}
	button.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBrand).Size(24, 24).TextColor(t.TextMuted)
				ui.Text(c, item.Name).FontSize(13).Bold().Grow(1).MinWidth(0).SingleLine().Clip().Tooltip(item.Name)
				if selected {
					ui.Text(c, "✓").TextColor(t.Accent)
				}
			})
			ui.Row(c).Gap(6).Wrap().Children(func() {
				pipeline := item.Pipeline
				if pipeline == "BUILT_IN" {
					pipeline = locales.PipelineBuiltin()
				}
				if pipeline != "" {
					ui.Badge(c, pipeline)
				}
				if item.Archive != "" {
					ui.Badge(c, locales.TemplateLocal())
				} else {
					ui.Text(c, humanSize(item.Size)).FontSize(11).TextColor(t.TextMuted)
				}
			})
		})
	})
}

func (a *app) projectTemplateDetails(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, locales.NewProjectName()).FontSize(12).Bold()
	ui.TextInput(c, &a.newName).Label(locales.NewProjectName()).Placeholder("My Game").Disabled(a.newCreating)
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, locales.NewProjectLocation()).FontSize(12).Bold()
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			label := a.newDir
			if label == "" {
				label = locales.NewProjectNotChosen()
			}
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine().Clip().Tooltip(label)
			ui.Button(c, locales.ActionChoose()).Disabled(a.newCreating).OnClick(func() { a.pickFolder(locales.DialogChooseFolder(), func(dir string) { a.newDir = dir }) })
		})
	})
	ui.Divider(c)
	if item, ok := a.selectedProjectTemplate(); ok {
		ui.Text(c, item.Name).FontSize(16).Bold()
		ui.Text(c, item.Description).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, item.ID+" · "+item.Version).FontSize(11).TextColor(t.TextMuted).SingleLine().Clip().Tooltip(item.key())
		if item.Archive != "" && item.URL != "" {
			ui.Button(c, locales.TemplateRepair()).Disabled(a.templateWorkBusy() || item.TermsURL != "" && !a.newTemplateTerms).OnClick(func() {
				if e, ok := a.editorInstall(a.newEditor); ok {
					a.startTemplateDownload(e, item)
				}
			})
		}
		if len(item.Platforms) > 0 {
			ui.Text(c, strings.Join(item.Platforms, ", ")).FontSize(11).TextColor(t.TextMuted)
		}
		if item.TermsURL != "" {
			ui.Button(c, locales.TemplateTerms()).OnClick(func() { _ = mygo.Shell.OpenExternal(item.TermsURL) })
			ui.Checkbox(c, &a.newTemplateTerms, locales.TemplateAcceptTerms())
		}
		if len(item.Packages) > 0 {
			ui.Text(c, locales.TemplatePackages(len(item.Packages))).FontSize(12).Bold()
			for _, p := range item.Packages {
				ui.Text(c, p.Name+"  "+p.Version).FontSize(11).TextColor(t.TextMuted)
			}
		}
	}
}

func (a *app) templateActivityBanner(c *ui.Context) {
	if a.templateJob == nil || a.page == pageActivity || a.newOpen {
		return
	}
	job := a.templateJob
	ui.Row(c).Padding(8, 24).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, locales.TemplateTask()+" · "+job.Template.Name).FontSize(12).Grow(1).MinWidth(0).SingleLine().Clip()
		if job.Busy {
			fraction := -1.0
			if job.Total > 0 {
				fraction = min(1, float64(job.Done)/float64(job.Total))
			}
			ui.Progress(c, fraction).Width(130)
		}
		ui.Button(c, locales.ActivityViewAll()).OnClick(func() { a.page = pageActivity })
	})
}

func (a *app) templateActivityCard(c *ui.Context) {
	job := a.templateJob
	if job == nil {
		return
	}
	c.Key("template-download")
	panel(c, func() {
		ui.Text(c, locales.TemplateTask()+" · "+job.Template.Name).FontSize(14).Bold()
		ui.Text(c, channelName(job.Editor.Kind)+" · "+displayVersion(job.Editor.Version, job.Editor.DisplayVersion)).FontSize(11).TextColor(c.Theme().TextMuted)
		if job.Busy {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				fraction := -1.0
				if job.Total > 0 {
					fraction = min(1, float64(job.Done)/float64(job.Total))
				}
				ui.Progress(c, fraction).Grow(1)
				ui.Text(c, humanSize(job.Done)+" / "+humanSize(job.Total)).FontSize(11)
				ui.Button(c, locales.ActionCancel()).OnClick(job.cancel)
			})
		} else {
			status := locales.TemplateDownloaded()
			if job.Err != "" {
				status = locales.TemplateDownloadFailed(job.Err)
			}
			ui.Text(c, status).FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Children(func() {
				if job.Err != "" {
					ui.Button(c, locales.ActionRetryDownload()).OnClick(func() { a.startTemplateDownload(job.Editor, job.Template) })
				}
				ui.Button(c, locales.TemplateView()).OnClick(func() {
					a.newOpen = true
					a.newEngine = job.Editor.Kind
					a.newEditor = job.Editor.Exe
					if a.newDir == "" {
						a.newDir = a.defaultProjectDir()
					}
					a.loadProjectTemplates(job.Editor, false)
					a.newTemplateID = job.Template.key()
					a.newTemplateChosen = true
				})
				ui.Button(c, locales.DialogOK()).OnClick(func() { a.templateJob = nil })
			})
		}
	})
}
