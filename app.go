package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

const (
	pageProjects = "/projects"
	pageInstalls = "/installs"
	pageSettings = "/settings"
)

// app is the state of the window: what was found on this machine, the page
// shown, and the dialog being filled.
type app struct {
	win   *mygo.Window
	page  string
	query string

	editors  []editorInstall
	projects []projectEntry
	prefs    prefs
	scanning bool

	list     ui.ListState
	installs ui.ListState

	newOpen   bool
	newDir    string
	newName   string
	newEditor string
	newError  string
}

func newApp() *app {
	return &app{page: pageProjects}
}

// load reads what the last run saved, and shows the language it chose.
func (a *app) load() {
	a.prefs = loadPrefs()
	if a.prefs.Locale != "" {
		mygo.App.SetLocale(a.prefs.Locale)
	}
}

// refresh scans the machine again, off the main thread.
func (a *app) refresh() {
	if a.scanning || a.win == nil {
		return
	}
	a.scanning = true
	settings := a.prefs
	go func() {
		editors, projects := scan(settings)
		a.win.Update(func() {
			a.editors, a.projects = editors, projects
			a.scanning = false
		})
	}()
}

func (a *app) save() {
	if err := savePrefs(a.prefs); err != nil {
		a.alert(err.Error())
	}
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		// The title across the sidebar's top, as the hubs brand theirs.
		ui.Column(c).Fill().Width(240).Background(t.Surface).Children(func() {
			ui.Text(c, locales.AppTitle()).FontSize(16).Bold().Padding(22, 16, 8, 16)
			ui.Sidebar(c, &a.page, func() {
				ui.SidebarItem(c, pageProjects, iconProjects, locales.SidebarProjects()).Children(func() {
					ui.Badge(c, fmt.Sprint(len(a.projects)))
				})
				ui.SidebarItem(c, pageInstalls, iconInstalls, locales.SidebarInstalls()).Children(func() {
					ui.Badge(c, fmt.Sprint(len(a.editors)))
				})
				ui.SidebarItem(c, pageSettings, iconSettings, locales.SidebarSettings())
			}).Grow(1)
		})
		ui.Column(c).Grow(1).Fill().Children(func() {
			switch a.page {
			case pageInstalls:
				a.installsPage(c)
			case pageSettings:
				a.settingsPage(c)
			default:
				a.projectsPage(c)
			}
		})
	})
	a.newProjectDialog(c)
}

// header is the title of a page, with its count and its actions.
func (a *app) header(c *ui.Context, title, subtitle string, actions func()) {
	t := c.Theme()
	ui.Row(c).Padding(24, 26, 18, 26).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(24).Bold()
			ui.Text(c, subtitle).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Spacer(c)
		actions()
	})
}

func (a *app) emptyState(c *ui.Context, title, hint string) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(8).Padding(40).Children(func() {
		ui.Text(c, title).FontSize(16).Bold().TextColor(t.TextMuted)
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(420)
	})
}

func (a *app) formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if !mygo.App.IsReady() {
		return t.Format("2006-01-02")
	}
	formats := mygo.App.Region().FormatLocale(mygo.App.Locale())
	return i18n.FormatDate(formats, t, i18n.DateOptions{Skeleton: "yMMMd"})
}

// alert reports a problem, which needs no answer.
func (a *app) alert(message string) {
	win := a.win
	go func() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Parent:  win,
			Type:    mygo.MessageWarning,
			Message: message,
			Buttons: []string{locales.DialogOK()},
		})
	}()
}

// pickFolder asks for a folder, and runs fn with it on the main thread.
func (a *app) pickFolder(title string, fn func(string)) {
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: title, Directory: true})
		if err != nil || len(paths) == 0 {
			return
		}
		dir := paths[0]
		win.Update(func() { fn(dir) })
	}()
}

func (a *app) openProject(p projectEntry) {
	if p.Editor == "" {
		a.alert(locales.EditorMissing(p.Version))
		return
	}
	if err := startDetached(p.Editor, []string{"-projectPath", p.Path}, filepath.Dir(p.Editor)); err != nil {
		a.alert(locales.OpenFailed(err.Error()))
	}
}

func (a *app) addProject() {
	a.pickFolder(locales.DialogAddProject(), func(dir string) {
		if !isProject(dir) {
			a.alert(locales.NotAProject(dir))
			return
		}
		a.prefs.PinnedProjects = dedupe(append(a.prefs.PinnedProjects, dir))
		a.save()
		a.refresh()
	})
}
