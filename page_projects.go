package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) projectsPage(c *ui.Context) {
	engine := engineForPage(a.page)
	width, _ := c.Size()
	compact := width < 1100
	if a.queryEngine != engine {
		a.query = ""
		a.queryEngine = engine
	}
	items := a.filteredProjects()
	title := locales.ProjectsTitle()
	if favoritesPage(a.page) {
		title = locales.SidebarFavorites()
	}
	a.header(c, title, locales.ProjectsCount(len(items)), func() {
		ui.Button(c, locales.ActionRefresh()).OnClick(a.refresh).Disabled(a.scanning)
		ui.Button(c, locales.ActionAddProject()).OnClick(func() { a.addProjectForEngine(engine) })
		ui.PrimaryButton(c, locales.ActionNewProject()).OnClick(a.openNewProject)
	})
	ui.Row(c).Padding(0, 24, 16, 24).Justify(ui.End).Children(func() {
		ui.SearchField(c, &a.query).Placeholder(locales.ProjectsSearch()).Label(locales.ProjectsSearch()).Width(245)
	})
	ui.Divider(c)
	for _, pending := range a.pendingProjectList {
		p := pending
		if p.Kind != engine {
			continue
		}
		c.Key(p.file)
		ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
			label := locales.NewProjectWaiting() + " · " + p.Path
			color := c.Theme().TextMuted
			if pendingProjectDelayed(p, time.Now()) {
				label = locales.NewProjectWaitingLong() + " · " + p.Path
			}
			if err := a.pendingProjectErrors[p.file]; err != "" {
				label = locales.NewProjectRegisterFailed() + " · " + p.Path + ": " + err
				color = c.Theme().Danger
			}
			ui.Text(c, label).Grow(1).FontSize(12).TextColor(color).Clip().Tooltip(label)
			if pendingProjectDelayed(p, time.Now()) {
				ui.Button(c, locales.NewProjectEditorLog()).OnClick(func() {
					path := editorLogPath(p.Kind)
					if path == "" {
						a.alert(locales.NewProjectLogMissing())
						return
					}
					if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
						a.alert(locales.NewProjectLogMissing())
						return
					}
					if err := mygo.Shell.OpenPath(path); err != nil {
						a.alert(err.Error())
					}
				})
			}
			ui.Button(c, locales.NewProjectForget()).OnClick(func() { a.forgetPendingProject(p) })
		})
	}
	ui.Row(c).Padding(4, 16).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		tableHeading(c, "★", "favorite", a.projectSort == "favorite", a.projectSortReverse, func() { a.sortProjects("favorite") }).Width(38)
		tableHeading(c, locales.ProjectColumnName(), "name", a.projectSort == "name", a.projectSortReverse, func() { a.sortProjects("name") }).Grow(1).MinWidth(0)
		dateWidth, versionWidth, actionsWidth := float32(110), float32(150), float32(190)
		if compact {
			dateWidth, versionWidth, actionsWidth = 76, 132, 104
		}
		tableHeading(c, locales.ProjectColumnModified(), "modified", a.projectSort == "modified" || a.projectSort == "", a.projectSortReverse, func() { a.sortProjects("modified") }).Width(dateWidth)
		tableHeading(c, locales.ProjectColumnEditor(), "version", a.projectSort == "version", a.projectSortReverse, func() { a.sortProjects("version") }).Width(versionWidth)
		ui.Text(c, "").Width(actionsWidth)
	})

	a.list.Key = func(i int) any { return items[i].Path }
	ui.List(c, &a.list, len(items), func(i int) {
		a.projectRow(c, items[i], compact)
	}).Grow(1).Children(func() {
		if len(items) == 0 {
			if strings.TrimSpace(a.query) != "" {
				a.searchEmpty(c, func() { a.query = "" })
			} else if favoritesPage(a.page) {
				a.emptyState(c, locales.FavoritesEmptyTitle(), locales.FavoritesEmptyHint())
			} else {
				a.emptyState(c, locales.ProjectsEmptyTitle(), locales.ProjectsEmptyHint())
			}
		}
	})
}

func (a *app) filteredProjects() []projectEntry {
	q := strings.ToLower(strings.TrimSpace(a.query))
	out := make([]projectEntry, 0, len(a.projects))
	for _, p := range a.projects {
		if p.Kind != engineForPage(a.page) || a.projectHidden(p.Path) || favoritesPage(a.page) && !p.Favorite {
			continue
		}
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Path), q) ||
			strings.Contains(strings.ToLower(p.Version), q) || strings.Contains(strings.ToLower(p.DisplayVersion), q) {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		var cmp int
		switch a.projectSort {
		case "name":
			cmp = strings.Compare(strings.ToLower(out[i].Name), strings.ToLower(out[j].Name))
		case "version":
			cmp = -compareVersions(out[i].Version, out[j].Version)
		case "favorite":
			if out[i].Favorite != out[j].Favorite {
				if out[i].Favorite {
					cmp = -1
				} else {
					cmp = 1
				}
			}
		default:
			if out[i].Modified.After(out[j].Modified) {
				cmp = -1
			} else if out[i].Modified.Before(out[j].Modified) {
				cmp = 1
			}
		}
		if cmp == 0 {
			cmp = strings.Compare(strings.ToLower(out[i].Path), strings.ToLower(out[j].Path))
		}
		if a.projectSortReverse {
			return cmp > 0
		}
		return cmp < 0
	})
	return out
}

func (a *app) sortProjects(column string) {
	if a.projectSort == column || a.projectSort == "" && column == "modified" {
		a.projectSort, a.projectSortReverse = column, !a.projectSortReverse
	} else {
		a.projectSort, a.projectSortReverse = column, false
	}
}

func (a *app) projectRow(c *ui.Context, p projectEntry, compact bool) {
	t := c.Theme()
	menu := func(m *ui.Menu) {
		if compact && m.Item(locales.ActionFolder()).Chosen() {
			mygo.Shell.OpenPath(p.Path)
		}
		if p.VCS.Remote != "" && isWebURL(p.VCS.Remote) && m.Item(locales.ActionRepo()).Chosen() {
			_ = mygo.Shell.OpenExternal(p.VCS.Remote)
		}
		if m.Item(locales.ProjectHide()).Chosen() {
			a.setProjectHidden(p.Path, true)
		}
	}
	row := ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		star := "☆"
		tip := locales.FavoriteAdd()
		if p.Favorite {
			star, tip = "★", locales.FavoriteRemove()
		}
		ui.Button(c, star).Tooltip(tip).Label(tip).Width(38).Background(ui.Color{}).Border(0, ui.Color{}).OnClick(func() { a.toggleFavorite(p) })
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Clip().Gap(5).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Name).Grow(1).MinWidth(0).FontSize(14).Bold().SingleLine().Clip().Tooltip(p.Name)
				if p.Pipeline != "" {
					ui.Badge(c, pipelineName(p.Pipeline))
				}
				if p.VCS.Kind != "" && !compact {
					badge := strings.ToUpper(p.VCS.Kind)
					if p.VCS.Branch != "" {
						badge += " · " + p.VCS.Branch
					}
					if p.VCS.Dirty {
						badge += " •"
					}
					b := ui.Badge(c, badge).FontSize(11).MaxWidth(130).Clip().Tooltip(badge)
					if p.VCS.Dirty {
						b.Tooltip(locales.VcsDirty())
					}
				}
			})
			ui.Text(c, p.Path).FontSize(12).TextColor(t.TextMuted).SingleLine().Clip().Tooltip(p.Path)
		})
		dateWidth, versionWidth, actionsWidth := float32(110), float32(150), float32(190)
		if compact {
			dateWidth, versionWidth, actionsWidth = 76, 132, 104
		}
		date := a.formatTime(p.Modified)
		ui.Text(c, date).Width(dateWidth).FontSize(11).TextColor(t.TextMuted).SingleLine().Clip().Tooltip(date)
		label := p.Version
		if p.DisplayVersion != "" {
			label = p.DisplayVersion
		}
		versionButton := ui.Button(c, label+" ▾").Width(versionWidth).Tooltip(displayVersion(p.Version, p.DisplayVersion)).OnClick(func() { a.chooseProjectVersion(p) })
		if p.Editor == "" {
			versionButton.TextColor(t.Warning).Tooltip(locales.EditorMissing(p.Version))
		}
		ui.Row(c).Gap(6).Width(actionsWidth).Justify(ui.End).Children(func() {
			ui.Button(c, locales.ActionOpen()).OnClick(func() {
				if p.Editor == "" || a.editorIsSimulated(p.Editor) {
					a.chooseProjectVersion(p)
				} else {
					a.openProject(p)
				}
			})
			if !compact {
				if ui.Button(c, locales.ActionFolder()).Clicked() {
					mygo.Shell.OpenPath(p.Path)
				}
			}
			ui.Button(c, "⋯").Width(36).Label(locales.ProjectMore()).Tooltip(locales.ProjectMore()).PopoverMenuButton(menu)
		})
	})
	row.PopoverMenu(menu)
	// A double click on the row opens the project too, as the hubs do.
	if row.RowDoubleClicked() {
		if p.Editor == "" || a.editorIsSimulated(p.Editor) {
			a.chooseProjectVersion(p)
		} else {
			a.openProject(p)
		}
	}
}

func (a *app) projectHidden(path string) bool {
	_, ok := a.prefs.HiddenProjects[localProjectKey(path)]
	return ok
}

func (a *app) setProjectHidden(path string, hide bool) {
	key := localProjectKey(path)
	if a.prefs.HiddenProjects == nil {
		a.prefs.HiddenProjects = map[string]string{}
	}
	previous := make(map[string]string, len(a.prefs.HiddenProjects))
	for stored, original := range a.prefs.HiddenProjects {
		previous[stored] = original
	}
	if hide {
		forgetProjectAliases(a.prefs.HiddenProjects, path)
		a.prefs.HiddenProjects[key] = filepath.Clean(path)
	} else {
		forgetProjectAliases(a.prefs.HiddenProjects, path)
	}
	if err := savePrefs(a.prefs); err != nil {
		a.prefs.HiddenProjects = previous
		a.alert(err.Error())
		return
	}
	a.refresh()
}

func (a *app) projectEditors(p projectEntry) []editorInstall {
	var editors []editorInstall
	for _, e := range a.editorsForEngine(p.Kind) {
		if !e.Simulated && !(a.uninstallBusy && strings.EqualFold(e.Exe, a.manageEditor.Exe)) {
			editors = append(editors, e)
		}
	}
	return editors
}

func (a *app) chooseProjectVersion(p projectEntry) {
	a.versionProject, a.versionOpen, a.versionConfirm, a.versionEditor = p, true, false, ""
	a.versionConfirmedEditor = editorInstall{}
	editors := a.projectEditors(p)
	if len(editors) > 0 {
		a.versionEditor = editors[0].Exe
	}
	for _, e := range editors {
		if strings.EqualFold(e.Version, p.Version) {
			a.versionEditor = e.Exe
			break
		}
	}
}

func (a *app) projectVersionDialog(c *ui.Context) {
	if !a.versionOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &a.versionOpen, func() {
		ui.Column(c).Width(500).Gap(14).Children(func() {
			if a.versionConfirm {
				ui.Text(c, locales.ProjectVersionConfirm()).FontSize(18).Bold()
				e := a.versionConfirmedEditor
				label := a.versionProject.Name + " · " + a.versionProject.Version + " → " + e.Version
				ui.Text(c, label).FontSize(14).Bold().SingleLine().Clip().Tooltip(label)
				ui.Text(c, e.Exe).FontSize(11).TextColor(t.TextMuted).SingleLine().Clip().Tooltip(e.Exe)
				ui.Text(c, locales.ProjectVersionWarning()).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, locales.ActionCancel()).OnClick(func() { a.versionConfirm = false })
					ui.PrimaryButton(c, locales.ProjectVersionContinue()).OnClick(a.openSelectedProjectVersion)
				})
				return
			}
			ui.Text(c, locales.ProjectVersionTitle()).FontSize(18).Bold()
			ui.Text(c, a.versionProject.Name+" · "+a.versionProject.Version).FontSize(13).SingleLine().Clip().Tooltip(a.versionProject.Name)
			ui.Text(c, locales.ProjectVersionHint()).FontSize(12).TextColor(t.TextMuted)
			editors := a.projectEditors(a.versionProject)
			if len(editors) == 0 {
				ui.Text(c, locales.ProjectVersionNone()).TextColor(t.TextMuted)
			}
			ui.Scroll(c).Height(200).Children(func() {
				ui.Column(c).Gap(5).Children(func() {
					for _, editor := range editors {
						e := editor
						c.Key(e.Exe)
						ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
							if strings.EqualFold(a.versionEditor, e.Exe) {
								ui.PrimaryButton(c, displayVersion(e.Version, e.DisplayVersion)).Width(200).OnClick(func() { a.versionEditor = e.Exe })
							} else {
								ui.Button(c, displayVersion(e.Version, e.DisplayVersion)).Width(200).OnClick(func() { a.versionEditor = e.Exe })
							}
							ui.Text(c, e.Exe).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine().Clip().Tooltip(e.Exe)
						})
					}
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.ProjectVersionInstall()).OnClick(func() {
					_, _, downloads, _ := enginePages(a.versionProject.Kind)
					a.page = downloads
					a.activeEngine = a.versionProject.Kind
					a.versionOpen = false
				})
				ui.Button(c, locales.ActionCancel()).OnClick(func() { a.versionOpen = false })
				ui.PrimaryButton(c, locales.ProjectVersionOpen()).Disabled(a.versionEditor == "").OnClick(func() {
					e, ok := a.editorInstall(a.versionEditor)
					if !ok || e.Kind != a.versionProject.Kind || e.Simulated {
						return
					}
					if strings.EqualFold(e.Version, a.versionProject.Version) {
						a.openSelectedProjectVersion()
					} else {
						a.versionConfirmedEditor = e
						a.versionConfirm = true
					}
				})
			})
		})
	})
}

func (a *app) openSelectedProjectVersion() {
	e, ok := a.editorInstall(a.versionEditor)
	if !ok || e.Kind != a.versionProject.Kind || e.Simulated || (a.versionConfirm && (!strings.EqualFold(e.Exe, a.versionConfirmedEditor.Exe) || !strings.EqualFold(e.Version, a.versionConfirmedEditor.Version) || e.Kind != a.versionConfirmedEditor.Kind)) {
		a.versionConfirm = false
		a.alert(locales.ProjectEditorChanged())
		a.refresh()
		return
	}
	p := a.versionProject
	p.Editor = e.Exe
	a.versionOpen = false
	a.openProjectWithEditor(p, e)
}

// pipelineName is the badge of a render pipeline.
func pipelineName(pipeline string) string {
	switch pipeline {
	case "urp":
		return "URP"
	case "hdrp":
		return "HDRP"
	default:
		return locales.PipelineBuiltin()
	}
}

// isWebURL reports whether a repository URL can be opened in a browser.
func isWebURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

func channelName(kind string) string {
	if kind == kindTuanjie {
		return locales.ChannelTuanjie()
	}
	return locales.ChannelUnity()
}

// editorLabels names each installed editor, and returns the executables in
// the same order.
func (a *app) editorLabels() ([]string, []string) {
	labels := make([]string, 0, len(a.editors))
	exes := make([]string, 0, len(a.editors))
	for _, e := range a.editorsForEngine(a.newEngine) {
		if e.Simulated || (a.uninstallBusy && strings.EqualFold(e.Exe, a.manageEditor.Exe)) {
			continue
		}
		labels = append(labels, e.Version+" · "+channelName(e.Kind))
		exes = append(exes, e.Exe)
	}
	counts := make(map[string]int, len(labels))
	for _, label := range labels {
		counts[label]++
	}
	for i, label := range labels {
		if counts[label] > 1 {
			labels[i] += " · " + exes[i]
		}
	}
	return labels, exes
}

func indexOf(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return 0
}

func (a *app) openNewProject() {
	if a.templateWorkBusy() {
		a.newOpen = true
		return
	}
	a.newEngine = engineForPage(a.page)
	_, exes := a.editorLabels()
	if len(exes) == 0 {
		a.alert(locales.NoEditorInstalled())
		return
	}
	a.newOpen = true
	a.newError = ""
	a.newName = ""
	a.newDir = a.defaultProjectDir()
	a.newEditor = exes[0]
	a.newTemplateQuery, a.newTemplateCategory = "", ""
	a.newTemplateID, a.newTemplateSource, a.newTemplatesErr = "", "", ""
	a.newTemplateChosen, a.newTemplateTerms = false, false
	a.newTemplates = nil
	if e, ok := a.editorInstall(a.newEditor); ok {
		a.loadProjectTemplates(e, false)
	}
}

func (a *app) defaultProjectDir() string {
	for _, root := range dedupe(append(a.prefs.ProjectRoots, defaultProjectRoots()...)) {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// createProject has the chosen editor make the project, and opens it.
func (a *app) createProject() {
	name := strings.TrimSpace(a.newName)
	if name == "" || a.newDir == "" || a.newEditor == "" {
		a.newError = locales.NewProjectIncomplete()
		return
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\:`) || !safeArchiveComponent(name) {
		a.newError = locales.NewProjectInvalidName()
		return
	}
	editor, ok := a.editorInstall(a.newEditor)
	if !ok || editor.Kind != a.newEngine || editor.Simulated {
		a.newError = locales.NoEditorInstalled()
		return
	}
	dir := filepath.Join(a.newDir, name)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		a.newError = locales.NewProjectExists(dir)
		return
	}
	template, ok := a.selectedProjectTemplate()
	if !ok || template.Archive == "" || a.newTemplateSource != templateEditorKey(editor) {
		a.newError = locales.TemplateChooseFirst()
		return
	}
	if template.TermsURL != "" && !a.newTemplateTerms {
		a.newError = locales.TemplateTermsRequired()
		return
	}
	if a.newCreating {
		return
	}
	launch := a.launchEditor
	finish := func(err error) {
		a.newCreating = false
		if err != nil {
			a.newError = locales.OpenFailed(err.Error())
			if !a.newOpen {
				a.alert(a.newError)
			}
			return
		}
		if abs, err := filepath.Abs(dir); err == nil {
			a.pendingProjectList = append(a.pendingProjectList, pendingProject{Path: abs, Version: editor.Version, Kind: editor.Kind, CreatedAt: time.Now(), file: pendingProjectFile(abs)})
		}
		a.newOpen = false
	}
	if a.win == nil {
		finish(startProjectCreationFromTemplate(dir, editor, template, launch))
		return
	}
	a.newCreating = true
	win := a.win
	go func() {
		err := startProjectCreationFromTemplate(dir, editor, template, launch)
		win.Update(func() { finish(err) })
	}()
}
