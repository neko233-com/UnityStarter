package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

const (
	pageProjects         = "/projects"
	pageFavorites        = "/favorites"
	pageInstalls         = "/installs"
	pageDownloads        = "/downloads"
	pageTuanjieProjects  = "/tuanjie/projects"
	pageTuanjieFavorites = "/tuanjie/favorites"
	pageTuanjieInstalls  = "/tuanjie/installs"
	pageTuanjieDownloads = "/tuanjie/downloads"
	pageStore            = "/store"
	pagePublish          = "/publish"
	pageLicense          = "/license"
	pageDocs             = "/docs"
	pageLearn            = "/learn"
	pageFeedback         = "/feedback"
	pageSettings         = "/settings"

	// appVersion is what the CLI prints and what the window shows when MyGo
	// does not know the packaged version.
	appVersion = "1.6.0"
	// appName is the name of the executable and of the app's data folder, as
	// mygo.json names the app; the window shows a name with a space.
	appName = "UnityStarter"
)

// app is the state of the window: what was found on this machine, the page
// shown, and the dialog being filled.
type app struct {
	win                *mygo.Window
	page               string
	query              string
	queryEngine        string
	activeEngine       string
	projectSort        string
	projectSortReverse bool
	installQuery       string
	installFilter      string
	versionOpen        bool
	versionConfirm     bool
	versionProject     projectEntry
	versionEditor      string
	launchEditor       func(string, []string, string) error
	manageOpen         bool
	manageEditor       editorInstall
	manageModules      bool
	manageUninstall    bool
	moduleBusy         bool
	moduleLoading      bool
	moduleInstalling   bool
	moduleStatus       string
	moduleCancel       context.CancelFunc

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

	// downloads is what the engines offer to install, read from the caches
	// and from Unity's releases API.
	downloads      []downloadEntry
	downloadsSince time.Time
	downloadsBusy  bool
	downloadsReady bool
	downloadsErr   string
	// jobs are the editors being downloaded or installed, by engine and
	// version.
	jobs          map[string]*downloadJob
	updateBusy    bool
	updatePending string
	updateStatus  string
	updateCtx     context.Context
	updateCancel  context.CancelFunc

	newOpen   bool
	newDir    string
	newName   string
	newEditor string
	newEngine string
	newError  string
}

func newApp() *app {
	return &app{page: pageProjects, activeEngine: kindUnity, launchEditor: startDetached}
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
		started := time.Now()
		editors, projects, hubs := scan(settings)
		logger.Printf("scan: %d editors, %d projects, %d hubs in %s",
			len(editors), len(projects), len(hubs), time.Since(started).Round(time.Millisecond))
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
	c.SetTheme(starterTheme)
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Fill().Width(68).Background(t.Surface).Padding(10, 4).Gap(8).Children(func() {
			a.engineTab(c, kindUnity)
			a.engineTab(c, kindTuanjie)
		})
		ui.Column(c).Fill().Width(220).Background(t.Surface).Children(func() {
			ui.Row(c).Padding(20, 16, 16, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBrand).Size(24, 24).TextColor(t.Text)
				ui.Text(c, channelName(a.activeEngine)).FontSize(15).Bold()
			})
			ui.Sidebar(c, &a.page, func() {
				projects, installs, downloads, favorites := enginePages(a.activeEngine)
				a.engineNavigation(c, a.activeEngine, projects, installs, downloads, favorites)
				ui.SidebarSection(c, locales.SidebarServices(), nil, func() {
					ui.SidebarItem(c, pageStore, iconStore, locales.PageStore())
					ui.SidebarItem(c, pagePublish, iconPublish, locales.PagePublish())
					ui.SidebarItem(c, pageLicense, iconLicense, locales.PageLicense())
				})
				ui.SidebarSection(c, locales.SidebarHelp(), nil, func() {
					ui.SidebarItem(c, pageDocs, iconDocs, locales.PageDocs())
					ui.SidebarItem(c, pageLearn, iconLearn, locales.PageLearn())
					ui.SidebarItem(c, pageFeedback, iconFeedback, locales.PageFeedback())
				})
				ui.SidebarItem(c, pageSettings, iconSettings, locales.SidebarSettings())
			}).Grow(1)
		})
		ui.Column(c).Grow(1).Fill().Children(func() {
			a.updateBanner(c)
			switch a.page {
			case pageProjects, pageFavorites, pageTuanjieProjects, pageTuanjieFavorites:
				a.projectsPage(c)
			case pageInstalls, pageTuanjieInstalls:
				a.installsPage(c)
			case pageDownloads, pageTuanjieDownloads:
				a.downloadsPage(c)
			case pageStore:
				a.storePage(c)
			case pagePublish:
				a.publishPage(c)
			case pageLicense:
				a.licensePage(c)
			case pageDocs:
				a.docsPage(c)
			case pageLearn:
				a.learnPage(c)
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
	a.projectVersionDialog(c)
	a.editorManageDialog(c)
}

func enginePages(engine string) (string, string, string, string) {
	if engine == kindTuanjie {
		return pageTuanjieProjects, pageTuanjieInstalls, pageTuanjieDownloads, pageTuanjieFavorites
	}
	return pageProjects, pageInstalls, pageDownloads, pageFavorites
}

func (a *app) engineTab(c *ui.Context, engine string) {
	label := "U"
	if engine == kindTuanjie {
		label = "T"
	}
	ui.Column(c).Gap(3).AlignItems(ui.Center).Children(func() {
		if engine == a.activeEngine {
			ui.PrimaryButton(c, label).Width(60).Tooltip(channelName(engine)).OnClick(func() { a.switchEngine(engine) })
		} else {
			ui.Button(c, label).Width(60).Tooltip(channelName(engine)).OnClick(func() { a.switchEngine(engine) })
		}
		ui.Text(c, channelName(engine)).Width(60).FontSize(10).TextColor(c.Theme().TextMuted).TextAlign(ui.Center).Clip()
	})
}

func (a *app) switchEngine(engine string) {
	if engine == a.activeEngine {
		return
	}
	oldProjects, oldInstalls, oldDownloads, oldFavorites := enginePages(a.activeEngine)
	projects, installs, downloads, favorites := enginePages(engine)
	switch a.page {
	case oldInstalls:
		a.page = installs
	case oldDownloads:
		a.page = downloads
	case oldFavorites:
		a.page = favorites
	case oldProjects:
		a.page = projects
	default:
		a.page = projects
	}
	a.activeEngine = engine
}

func engineForPage(page string) string {
	if strings.HasPrefix(page, "/tuanjie/") {
		return kindTuanjie
	}
	return kindUnity
}

func favoritesPage(page string) bool { return page == pageFavorites || page == pageTuanjieFavorites }

func (a *app) engineNavigation(c *ui.Context, engine, projects, installs, downloads, favorites string) {
	ui.SidebarSection(c, locales.SidebarWorkspace(), nil, func() {
		ui.SidebarItem(c, projects, iconProjects, locales.SidebarProjects()).Children(func() {
			count := 0
			for _, p := range a.projects {
				if p.Kind == engine {
					count++
				}
			}
			ui.Badge(c, fmt.Sprint(count))
		})
		ui.SidebarItem(c, installs, iconInstalls, locales.SidebarInstalls()).Children(func() {
			count := 0
			for _, e := range a.editors {
				if e.Kind == engine {
					count++
				}
			}
			ui.Badge(c, fmt.Sprint(count))
		})
		ui.SidebarItem(c, downloads, iconDownload, locales.SidebarDownloads())
		ui.SidebarItem(c, favorites, iconFavorite, locales.SidebarFavorites())
	})
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
	logger.Printf("alert: %s", message)
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
	for _, e := range a.editors {
		if strings.EqualFold(e.Exe, p.Editor) && e.Simulated {
			a.alert(locales.SimulatedOpen())
			return
		}
	}
	if err := a.launchEditor(p.Editor, []string{"-projectPath", p.Path}, filepath.Dir(p.Editor)); err != nil {
		a.alert(locales.OpenFailed(err.Error()))
	}
}

func (a *app) addProject() {
	a.addProjectForEngine("")
}

func (a *app) addProjectForEngine(engine string) {
	a.pickFolder(locales.DialogAddProject(), func(dir string) {
		if !isProject(dir) {
			a.alert(locales.NotAProject(dir))
			return
		}
		if version, _, ok := readProject(dir); engine != "" && ok && kindOfVersion(version) != engine {
			a.alert(locales.ProjectWrongEngine())
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
	if a.prefs.NoHubSync {
		return hub{}, false
	}
	for _, h := range a.hubs {
		if h.Kind == p.Kind {
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
	if h.Kind != p.Hub && h.writable() {
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
		a.projects[i].Hub = h.Kind
	}
}

// pushToHub adds a project this app added to the hub that owns its engine,
// so that the hub lists it too.
func (a *app) pushToHub(dir string) {
	if a.prefs.NoHubSync {
		return
	}
	version, info, ok := readProject(dir)
	if !ok {
		return
	}
	entry := projectEntry{Path: filepath.Clean(dir), Name: filepath.Base(dir), Version: version, Kind: kindOfVersion(version)}
	if info != nil {
		entry.Modified = info.ModTime()
	}
	added, err := exportProjects(a.hubs, []projectEntry{entry})
	if err != nil {
		a.alert(locales.HubWriteFailed(err.Error()))
		return
	}
	if added > 0 {
		a.hubWriteAt = time.Now()
	}
}

// editorInstall is the installed editor of an executable, whose version a
// project created with it has.
func (a *app) editorInstall(exe string) (editorInstall, bool) {
	for _, e := range a.editors {
		if strings.EqualFold(e.Exe, exe) {
			return e, true
		}
	}
	return editorInstall{}, false
}

// editorIsSimulated reports whether an executable is a stand-in.
func (a *app) editorIsSimulated(exe string) bool {
	e, ok := a.editorInstall(exe)
	return ok && e.Simulated
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
