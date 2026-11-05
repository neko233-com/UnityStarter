package main

import (
	"context"
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
	pageStore    = "/store"
	pagePublish  = "/publish"
	pageLicense  = "/license"
	pageDocs     = "/docs"
	pageFeedback = "/feedback"
	pageSettings = "/settings"

	// appVersion is what the CLI prints and what the window shows when MyGo
	// does not know the packaged version.
	appVersion = "0.1.0"
)

// app is the state of the window: what was found on this machine, the page
// shown, and the dialog being filled.
type app struct {
	win   *mygo.Window
	page  string
	query string

	editors  []editorInstall
	projects []projectEntry
	hubs     []hub
	prefs    prefs
	scanning bool

	// hubWriteAt is when this app last wrote a hub's files, so that the
	// change the write itself reports is not read as the hub's.
	hubWriteAt time.Time
	hubCancel  context.CancelFunc

	list     ui.ListState
	installs ui.ListState

	// diagCopied says the diagnostics were just copied, beside their button.
	diagCopied bool

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
		editors, projects, hubs := scan(settings)
		a.win.Update(func() {
			a.editors, a.projects, a.hubs = editors, projects, hubs
			a.scanning = false
		})
	}()
}

// hubChanged runs when a hub writes the files this app shares with it.
func (a *app) hubChanged() {
	if time.Since(a.hubWriteAt) < 1500*time.Millisecond {
		return
	}
	a.refresh()
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
				ui.SidebarSection(c, locales.SidebarWorkspace(), nil, func() {
					ui.SidebarItem(c, pageProjects, iconProjects, locales.SidebarProjects()).Children(func() {
						ui.Badge(c, fmt.Sprint(len(a.projects)))
					})
					ui.SidebarItem(c, pageInstalls, iconInstalls, locales.SidebarInstalls()).Children(func() {
						ui.Badge(c, fmt.Sprint(len(a.editors)))
					})
				})
				ui.SidebarSection(c, locales.SidebarServices(), nil, func() {
					ui.SidebarItem(c, pageStore, iconStore, locales.PageStore())
					ui.SidebarItem(c, pagePublish, iconPublish, locales.PagePublish())
					ui.SidebarItem(c, pageLicense, iconLicense, locales.PageLicense())
				})
				ui.SidebarSection(c, locales.SidebarHelp(), nil, func() {
					ui.SidebarItem(c, pageDocs, iconDocs, locales.PageDocs())
					ui.SidebarItem(c, pageFeedback, iconFeedback, locales.PageFeedback())
				})
				ui.SidebarItem(c, pageSettings, iconSettings, locales.SidebarSettings())
			}).Grow(1)
		})
		ui.Column(c).Grow(1).Fill().Children(func() {
			switch a.page {
			case pageInstalls:
				a.installsPage(c)
			case pageStore:
				a.storePage(c)
			case pagePublish:
				a.publishPage(c)
			case pageLicense:
				a.licensePage(c)
			case pageDocs:
				a.docsPage(c)
			case pageFeedback:
				a.feedbackPage(c)
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
		a.pushToHub(dir)
		a.refresh()
	})
}

// hubFor is the hub a project's star and additions go to: the one that
// lists it, else the Unity Hub, which holds a project list this app writes.
func (a *app) hubFor(p projectEntry) (hub, bool) {
	for _, h := range a.hubs {
		if h.Kind == p.Hub {
			return h, true
		}
	}
	if a.prefs.NoHubSync {
		return hub{}, false
	}
	for _, h := range a.hubs {
		if h.Kind == kindUnity && h.writable() {
			return h, true
		}
	}
	return hub{}, false
}

// toggleFavorite stars a project in its hub, or unstars it.
func (a *app) toggleFavorite(p projectEntry) {
	h, ok := a.hubFor(p)
	if !ok {
		if a.prefs.NoHubSync {
			a.alert(locales.HubSyncDisabled())
		} else {
			a.alert(locales.HubSyncOff())
		}
		return
	}
	on := !p.Favorite
	if h.Kind != p.Hub {
		if err := h.addProject(p); err != nil {
			a.alert(locales.HubWriteFailed(err.Error()))
			return
		}
	}
	if err := h.setFavorite(p.Path, on); err != nil {
		a.alert(locales.HubWriteFailed(err.Error()))
		return
	}
	a.hubWriteAt = time.Now()
	for i := range a.projects {
		if a.projects[i].Path != p.Path {
			continue
		}
		a.projects[i].Favorite = on
		if a.projects[i].Hub == "" {
			a.projects[i].Hub = h.Kind
		}
	}
}

// pushToHub adds a project this app added or created to the hub that owns
// its engine, so that the hub lists it too.
func (a *app) pushToHub(dir string) {
	if a.prefs.NoHubSync {
		return
	}
	version, info, ok := readProject(dir)
	if !ok {
		return
	}
	entry := projectEntry{Path: filepath.Clean(dir), Name: filepath.Base(dir), Version: version}
	if info != nil {
		entry.Modified = info.ModTime()
	}
	for _, h := range a.hubs {
		if !h.writable() || h.Kind != kindOfVersion(version) {
			continue
		}
		if err := h.addProject(entry); err != nil {
			a.alert(locales.HubWriteFailed(err.Error()))
			return
		}
		a.hubWriteAt = time.Now()
	}
}

// startHubWatch follows the hubs' files while the app runs, so that a change
// made in a hub shows here without a manual refresh.
func (a *app) startHubWatch() {
	hubs := detectHubs()
	if len(hubs) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.hubCancel = cancel
	watchHubs(ctx, hubs, func() {
		a.win.Update(a.hubChanged)
	})
}
