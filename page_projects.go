package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) projectsPage(c *ui.Context) {
	a.header(c, locales.ProjectsTitle(), locales.ProjectsCount(len(a.projects)), func() {
		ui.SearchField(c, &a.query).Placeholder(locales.ProjectsSearch()).Width(220)
		ui.Button(c, locales.ActionRefresh()).OnClick(a.refresh).Disabled(a.scanning)
		ui.Button(c, locales.ActionAddProject()).OnClick(a.addProject)
		ui.PrimaryButton(c, locales.ActionNewProject()).OnClick(a.openNewProject)
	})
	ui.Divider(c)

	items := a.filteredProjects()
	a.list.Key = func(i int) any { return items[i].Path }
	ui.List(c, &a.list, len(items), func(i int) {
		a.projectRow(c, items[i])
	}).Grow(1).Children(func() {
		if len(items) == 0 {
			a.emptyState(c, locales.ProjectsEmptyTitle(), locales.ProjectsEmptyHint())
		}
	})
}

func (a *app) filteredProjects() []projectEntry {
	q := strings.ToLower(strings.TrimSpace(a.query))
	if q == "" {
		return a.projects
	}
	out := make([]projectEntry, 0, len(a.projects))
	for _, p := range a.projects {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Path), q) ||
			strings.Contains(strings.ToLower(p.Version), q) {
			out = append(out, p)
		}
	}
	return out
}

func (a *app) projectRow(c *ui.Context, p projectEntry) {
	t := c.Theme()
	row := ui.Row(c).Padding(12, 18).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Name).FontSize(14).Bold().Clip()
				ui.Badge(c, channelName(p.Kind))
			})
			ui.Text(c, p.Path).FontSize(11).TextColor(t.TextMuted).Clip()
		})
		ui.Column(c).Width(150).Gap(3).Children(func() {
			ui.Text(c, p.Version).FontSize(12)
			ui.Text(c, locales.OpenedAt(a.formatTime(p.Modified))).FontSize(11).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(6).Children(func() {
			star := "☆"
			tip := locales.FavoriteAdd()
			if p.Favorite {
				star, tip = "★", locales.FavoriteRemove()
			}
			if ui.Button(c, star).Tooltip(tip).Width(38).Clicked() {
				a.toggleFavorite(p)
			}
			if p.Editor == "" {
				ui.Button(c, locales.ActionOpen()).Disabled(true).Tooltip(locales.EditorMissing(p.Version))
			} else if ui.PrimaryButton(c, locales.ActionOpen()).Clicked() {
				a.openProject(p)
			}
			if ui.Button(c, locales.ActionFolder()).Clicked() {
				mygo.Shell.OpenPath(p.Path)
			}
		})
	})
	// A double click on the row opens the project too, as the hubs do.
	if row.RowDoubleClicked() {
		a.openProject(p)
	}
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
	for _, e := range a.editors {
		labels = append(labels, e.Version+" · "+channelName(e.Kind))
		exes = append(exes, e.Exe)
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
	if len(a.editors) == 0 {
		a.alert(locales.NoEditorInstalled())
		return
	}
	a.newOpen = true
	a.newError = ""
	a.newName = ""
	a.newDir = a.defaultProjectDir()
	a.newEditor = a.editors[0].Exe
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
	dir := filepath.Join(a.newDir, name)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		a.newError = locales.NewProjectExists(dir)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.newError = err.Error()
		return
	}
	if err := startDetached(a.newEditor, []string{"-createProject", dir}, filepath.Dir(a.newEditor)); err != nil {
		a.newError = locales.OpenFailed(err.Error())
		return
	}
	a.prefs.PinnedProjects = dedupe(append(a.prefs.PinnedProjects, dir))
	a.save()
	a.newOpen = false
	a.pushToHub(dir)
	a.refresh()
}
