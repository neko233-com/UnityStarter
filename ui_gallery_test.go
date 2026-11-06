package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

// Exercise the actual native view at its minimum supported size. The footer
// must remain reachable even when the navigation above it needs scrolling.
func TestWorkspaceNavigationAtMinimumSize(t *testing.T) {
	a := fixture()
	tt := ui.NewTester(a.view, 900, 560)
	for _, route := range []struct{ label, page string }{
		{locales.SidebarSettings(), pageSettings}, {locales.ActivityTitle(), pageActivity},
		{locales.SidebarProjects(), pageProjects}, {locales.SidebarInstalls(), pageInstalls},
	} {
		bounds, ok := tt.Find(route.label)
		if !ok || bounds.Y < 36 || bounds.Y+bounds.H > 560 {
			t.Fatalf("navigation %q is outside the window: %+v", route.label, bounds)
		}
		if err := tt.Click(route.label); err != nil {
			t.Fatal(err)
		}
		if a.page != route.page {
			t.Fatalf("%q navigated to %q", route.label, a.page)
		}
	}
	tt.Scroll(180, 330, 0, 700)
	tt.Frame()
	for _, route := range []struct{ label, page string }{
		{locales.PageLearn(), pageLearn}, {locales.PageFeedback(), pageFeedback},
	} {
		bounds, ok := tt.Find(route.label)
		if !ok || bounds.Y < 36 || bounds.Y+bounds.H > 472 {
			t.Fatalf("scrolling did not reveal %q above the footer: %+v", route.label, bounds)
		}
		if err := tt.Click(route.label); err != nil {
			t.Fatal(err)
		}
		if a.page != route.page {
			t.Fatalf("%q navigated to %q", route.label, a.page)
		}
	}
	a.page = pageSettings
	tt.Frame()
	for _, tab := range []struct{ label, content string }{
		{locales.SettingsProjectsTab(), locales.SettingsProjectRoots()},
		{locales.SettingsInstallsTab(), locales.SettingsInstallRoot()},
		{locales.SettingsAbout(), locales.ActionCheckUpdate()},
		{locales.SettingsGeneral(), locales.SettingsLanguage()},
	} {
		if err := tt.Click(tab.label); err != nil {
			t.Fatal(err)
		}
		if !tt.HasText(tab.content) {
			t.Fatalf("settings tab %q lost its controls: %q", tab.label, tt.Texts())
		}
	}
	a.switchEngine(kindTuanjie)
	tt.Frame()
	if a.page != pageSettings || a.activeEngine != kindTuanjie {
		t.Fatal("switching engine should retain shared settings page")
	}
}

func TestCatalogFiltersAreIndependentAndKeepJobsVisible(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	preview := downloadEntry{Engine: kindUnity, Version: "7000.0.0a16", Stream: "ALPHA"}
	a.downloads = append(a.downloads, preview)
	tt := ui.NewTester(a.view, 900, 560)
	if err := tt.Click(locales.InstallPreview()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(preview.Version) || !tt.HasText("6000.3.28f1") || tt.HasText("6000.0.85f2") {
		t.Fatalf("preview filter must retain ongoing jobs, not unrelated releases: %q", tt.Texts())
	}
	a.switchEngine(kindTuanjie)
	tt.Frame()
	if a.downloadFilterForEngine(kindTuanjie) != "" || !tt.HasText("2022.3.62t17") {
		t.Fatal("Unity release filter leaked into Tuanjie")
	}
	a.switchEngine(kindUnity)
	tt.Frame()
	if a.downloadFilterForEngine(kindUnity) != "preview" {
		t.Fatal("Unity filter was not preserved")
	}
	a.page, a.installQuery = pageInstalls, "no such editor"
	tt.Frame()
	if err := tt.Click(locales.SearchClear()); err != nil {
		t.Fatal(err)
	}
	if a.installQuery != "" || !tt.HasText(a.editors[0].Version) {
		t.Fatal("clear filters did not restore installed Editors")
	}
}

func TestProjectDefaultSortTogglesOnFirstClick(t *testing.T) {
	a := fixture()
	a.jobs = nil
	a.projects = append(a.projects, projectEntry{Kind: kindUnity, Name: "Older project", Path: "older", Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
	tt := ui.NewTester(a.view, 900, 560)
	if a.filteredProjects()[0].Name != "demo" {
		t.Fatal("default sorting should put recent projects first")
	}
	if err := tt.Click(locales.ProjectColumnModified()); err != nil {
		t.Fatal(err)
	}
	if a.filteredProjects()[0].Name != "Older project" {
		t.Fatal("clicking the active default column should reverse its order immediately")
	}
}

func galleryFixture() *app {
	a := installedCardFixture()
	a.activeEngine = kindUnity
	a.editors = append(a.editors, editorInstall{Kind: kindUnity, Version: "6000.3.28f1", Exe: `D:\Unity\6000.3.28f1\Editor\Unity.exe`, Modules: a.editors[1].Modules[:3]})
	a.projects = append(a.projects, projectEntry{
		Path: `D:\Code\Games\Starfall`, Name: "Starfall", Kind: kindUnity, Version: "6000.3.28f1",
		Editor: a.editors[2].Exe, Modified: time.Now(), Pipeline: "urp", Favorite: true,
		VCS: vcsInfo{Kind: "git", Branch: "feature/world-map", Dirty: true},
	}, projectEntry{Path: `D:\Code\Experiments\LightingLab`, Name: "Lighting Lab", Kind: kindUnity, Version: "6000.0.85f2", Pipeline: "hdrp"})
	a.prefs.ProjectRoots = []string{`D:\Code\Games`}
	a.prefs.EditorRoots = []string{`D:\Unity`, `D:\IDE\TuanJieEditor`}
	a.prefs.HiddenProjects = map[string]string{"hidden": `D:\Code\Archive\Prototype`}
	a.downloads = append(a.downloads, downloadEntry{Engine: kindUnity, Version: "7000.0.0a16", Stream: "ALPHA", Size: 3892458520, Date: time.Now(), URL: "https://download.unity3d.com/unity/test/UnitySetup64.exe"})
	for i := range a.downloads {
		a.downloads[i].Size = 2892458520
	}
	a.jobs = map[string]*downloadJob{}
	e := a.downloads[1]
	a.jobs[jobKey(e)] = &downloadJob{Entry: e, Status: "downloading", Done: 1200000000, Total: e.Size, Speed: 16000000}
	e = a.downloads[2]
	a.jobs[jobKey(e)] = &downloadJob{Entry: e, Status: "failedDownload", Err: "Network connection was interrupted. The partial download is retained."}
	a.editors[2].OfflineDocs = `D:\Unity\6000.3.28f1\Documentation`
	return a
}

// Render every tab, including each settings category, for human inspection.
// This is offscreen rendering of the native view, not a live-window capture.
func TestWorkspaceGallery(t *testing.T) {
	dir := os.Getenv("UI_GALLERY")
	if dir == "" {
		t.Skip("set UI_GALLERY to save native offscreen previews")
	}
	old := i18n.Locale()
	locale := os.Getenv("UI_GALLERY_LOCALE")
	if locale == "" {
		locale = "zh-Hans"
	}
	i18n.SetLocaleFunc(func() string { return locale })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return old }) })
	for _, size := range []struct{ w, h int }{{1400, 820}, {900, 560}} {
		for _, page := range []string{pageProjects, pageFavorites, pageTuanjieProjects, pageTuanjieFavorites, pageInstalls, pageTuanjieInstalls, pageDownloads, pageTuanjieDownloads, pageActivity, pageStore, pagePublish, pageLicense, pageDocs, pageLearn, pageFeedback, pageSettings} {
			for _, tab := range []string{"", "projects", "installs", "about"} {
				if page != pageSettings && tab != "" {
					continue
				}
				a := galleryFixture()
				a.page, a.settingsTab = page, tab
				if strings.HasPrefix(page, "/tuanjie/") {
					a.activeEngine = kindTuanjie
				}
				tt := ui.NewTester(a.view, size.w, size.h)
				tt.SetTitleBar(ui.TitleBar{Height: 36, Right: 138})
				name := fmt.Sprintf("%s%s-%d.png", strings.ReplaceAll(strings.TrimPrefix(page, "/"), "/", "-"), tab, size.w)
				if err := writePNG(filepath.Join(dir, name), tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
