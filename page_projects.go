package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) projectsPage(c *ui.Context) {
	engine := engineForPage(a.page)
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
	ui.Row(c).Padding(0, 22, 14, 22).Justify(ui.End).Children(func() {
		ui.SearchField(c, &a.query).Placeholder(locales.ProjectsSearch()).Width(245)
	})
	ui.Divider(c)
	for _, pending := range a.pendingProjectList {
		p := pending
		if p.Kind != engine {
			continue
		}
		c.Key(p.file)
		ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
			ui.Text(c, locales.NewProjectWaiting()+" · "+p.Path).Grow(1).FontSize(12).Clip().Tooltip(p.Path)
			ui.Button(c, locales.NewProjectForget()).OnClick(func() { a.forgetPendingProject(p) })
		})
	}
	ui.Row(c).Padding(5, 16).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		ui.Button(c, "★").Width(38).OnClick(func() { a.sortProjects("favorite") })
		ui.Button(c, locales.ProjectColumnName()).Grow(1).OnClick(func() { a.sortProjects("name") })
		ui.Button(c, locales.ProjectColumnModified()).Width(110).OnClick(func() { a.sortProjects("modified") })
		ui.Button(c, locales.ProjectColumnEditor()).Width(150).OnClick(func() { a.sortProjects("version") })
		ui.Text(c, "").Width(190)
	})

	a.list.Key = func(i int) any { return items[i].Path }
	ui.List(c, &a.list, len(items), func(i int) {
		a.projectRow(c, items[i])
	}).Grow(1).Children(func() {
		if len(items) == 0 {
			if favoritesPage(a.page) && strings.TrimSpace(a.query) == "" {
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
	if a.projectSort == column {
		a.projectSortReverse = !a.projectSortReverse
	} else {
		a.projectSort, a.projectSortReverse = column, false
	}
}

func (a *app) projectRow(c *ui.Context, p projectEntry) {
	t := c.Theme()
	menu := func(m *ui.Menu) {
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
		ui.Button(c, star).Tooltip(tip).Width(38).OnClick(func() { a.toggleFavorite(p) })
		ui.Column(c).Grow(1).Gap(5).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Name).FontSize(16).Bold().Clip()
				ui.Badge(c, channelName(p.Kind))
				if p.Pipeline != "" {
					ui.Badge(c, pipelineName(p.Pipeline))
				}
				if p.VCS.Kind != "" {
					badge := strings.ToUpper(p.VCS.Kind)
					if p.VCS.Branch != "" {
						badge += " · " + p.VCS.Branch
					}
					if p.VCS.Dirty {
						badge += " •"
					}
					b := ui.Badge(c, badge).FontSize(11)
					if p.VCS.Dirty {
						b.Tooltip(locales.VcsDirty())
					}
				}
			})
			ui.Text(c, p.Path).FontSize(12).TextColor(t.TextMuted).Clip()
		})
		ui.Text(c, a.formatTime(p.Modified)).Width(110).FontSize(11).TextColor(t.TextMuted)
		label := p.Version
		if p.DisplayVersion != "" {
			label = p.DisplayVersion
		}
		ui.Button(c, label+" ▾").Width(150).Tooltip(displayVersion(p.Version, p.DisplayVersion)).OnClick(func() { a.chooseProjectVersion(p) })
		ui.Row(c).Gap(6).Width(190).Justify(ui.End).Children(func() {
			ui.Button(c, locales.ActionOpen()).OnClick(func() {
				if p.Editor == "" || a.editorIsSimulated(p.Editor) {
					a.chooseProjectVersion(p)
				} else {
					a.openProject(p)
				}
			})
			if ui.Button(c, locales.ActionFolder()).Clicked() {
				mygo.Shell.OpenPath(p.Path)
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
	previous, existed := a.prefs.HiddenProjects[key]
	if hide {
		a.prefs.HiddenProjects[key] = filepath.Clean(path)
	} else {
		delete(a.prefs.HiddenProjects, key)
	}
	if err := savePrefs(a.prefs); err != nil {
		if existed {
			a.prefs.HiddenProjects[key] = previous
		} else {
			delete(a.prefs.HiddenProjects, key)
		}
		a.alert(err.Error())
		return
	}
	a.refresh()
}

func (a *app) projectEditors(p projectEntry) []editorInstall {
	var editors []editorInstall
	for _, e := range a.editorsForEngine(p.Kind) {
		if !e.Simulated {
			editors = append(editors, e)
		}
	}
	return editors
}

func (a *app) chooseProjectVersion(p projectEntry) {
	a.versionProject, a.versionOpen, a.versionConfirm, a.versionEditor = p, true, false, ""
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
				e, ok := a.editorInstall(a.versionEditor)
				if !ok || e.Kind != a.versionProject.Kind || e.Simulated {
					ui.Text(c, locales.ProjectVersionNone()).TextColor(t.Danger)
					return
				}
				ui.Text(c, a.versionProject.Name+" · "+a.versionProject.Version+" → "+e.Version).FontSize(14).Bold()
				ui.Text(c, locales.ProjectVersionWarning()).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, locales.ActionCancel()).OnClick(func() { a.versionConfirm = false })
					ui.PrimaryButton(c, locales.ProjectVersionContinue()).OnClick(a.openSelectedProjectVersion)
				})
				return
			}
			ui.Text(c, locales.ProjectVersionTitle()).FontSize(18).Bold()
			ui.Text(c, a.versionProject.Name+" · "+a.versionProject.Version).FontSize(13)
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
							ui.Text(c, e.Exe).FontSize(11).TextColor(t.TextMuted).Grow(1).Clip()
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
						a.versionConfirm = true
					}
				})
			})
		})
	})
}

func (a *app) openSelectedProjectVersion() {
	e, ok := a.editorInstall(a.versionEditor)
	if !ok || e.Kind != a.versionProject.Kind || e.Simulated {
		return
	}
	p := a.versionProject
	p.Editor = e.Exe
	a.versionOpen = false
	a.openProject(p)
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
		if e.Simulated {
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

func (a *app) newProjectDialog(c *ui.Context) {
	if !a.newOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &a.newOpen, func() {
		ui.Column(c).Width(460).Gap(16).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.NewProjectTitle()).FontSize(18).Bold()
				ui.Text(c, locales.NewProjectHint()).FontSize(12).TextColor(t.TextMuted)
			})

			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, locales.NewProjectLocation()).FontSize(12).Bold()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					label := a.newDir
					if label == "" {
						label = locales.NewProjectNotChosen()
					}
					ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Grow(1).Clip()
					if ui.Button(c, locales.ActionChoose()).Clicked() {
						a.pickFolder(locales.DialogChooseFolder(), func(dir string) { a.newDir = dir })
					}
				})
			})

			ui.TextInput(c, &a.newName).Label(locales.NewProjectName()).Placeholder("My Game")

			labels, exes := a.editorLabels()
			if len(labels) > 0 {
				chosen := labels[indexOf(exes, a.newEditor)]
				ui.Select(c, &chosen, labels).Label(locales.NewProjectEditor())
				a.newEditor = exes[indexOf(labels, chosen)]
			}

			if a.newError != "" {
				ui.Text(c, a.newError).FontSize(12).TextColor(t.Danger)
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.ActionCancel()).Clicked() {
					a.newOpen = false
				}
				if ui.PrimaryButton(c, locales.ActionCreate()).Clicked() {
					a.createProject()
				}
			})
		})
	})
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
	if err := startProjectCreation(dir, editor, a.launchEditor); err != nil {
		a.newError = locales.OpenFailed(err.Error())
		logger.Printf("new project: %v", err)
		return
	}
	if abs, err := filepath.Abs(dir); err == nil {
		a.pendingProjectList = append(a.pendingProjectList, pendingProject{Path: abs, Version: editor.Version, Kind: editor.Kind, file: pendingProjectFile(abs)})
	}
	a.newOpen = false
}
