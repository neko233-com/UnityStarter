package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func fixture() *app {
	a := newApp()
	a.projects = []projectEntry{
		{
			Path:     `D:\Code\Games\demo`,
			Name:     "demo",
			Version:  "2022.3.51f1",
			Kind:     kindUnity,
			Modified: time.Date(2026, 1, 5, 13, 7, 0, 0, time.Local),
			Editor:   `C:\Program Files\Unity\Hub\Editor\2022.3.51f1\Editor\Unity.exe`,
			Hub:      kindUnity,
			Favorite: true,
			Pipeline: "urp",
			VCS:      vcsInfo{Kind: "git", Branch: "main", Remote: "https://github.com/neko233-com/UnityStarter"},
		},
		{
			Path:     `D:\Code\Games\cozy`,
			Name:     "cozy",
			Version:  "2022.3.62t17",
			Kind:     kindTuanjie,
			Modified: time.Date(2025, 12, 1, 9, 0, 0, 0, time.Local),
		},
	}
	a.editors = []editorInstall{
		{
			Version:   "2022.3.51f1",
			Kind:      kindUnity,
			Exe:       `C:\Program Files\Unity\Hub\Editor\2022.3.51f1\Editor\Unity.exe`,
			Root:      `C:\Program Files\Unity\Hub\Editor\2022.3.51f1`,
			Simulated: true,
		},
	}
	a.hubs = []hub{
		{
			Kind:      kindUnity,
			Dir:       `C:\Users\ada\AppData\Roaming\UnityHub`,
			Version:   "3.3.7",
			Registry:  `C:\Users\ada\AppData\Roaming\UnityHub\projects-v1.json`,
			Favorites: `C:\Users\ada\AppData\Roaming\UnityHub\favoriteProjects.json`,
		},
		{
			Kind:      kindTuanjie,
			Dir:       `C:\Users\ada\AppData\Roaming\TuanjieHub`,
			Version:   "1.4.7",
			Favorites: `C:\Users\ada\AppData\Roaming\TuanjieHub\favoriteProjects.json`,
		},
	}
	a.downloads = []downloadEntry{
		{Engine: kindUnity, Version: "6000.0.85f2", LTS: true, URL: "https://download.unity3d.com/x.exe", Size: 4096, Date: time.Date(2026, 11, 3, 16, 15, 0, 0, time.UTC)},
		{Engine: kindUnity, Version: "6000.3.28f1", LTS: true, URL: "https://download.unity3d.com/y.exe", Size: 4096},
		{Engine: kindTuanjie, Version: "2022.3.62t17", LTS: true, URL: "https://download.tuanjie.cn/z.exe", Size: 4096},
	}
	a.downloadsReady = true
	a.downloadsSince = time.Now()
	a.jobs = map[string]*downloadJob{
		jobKey(a.downloads[1]): {
			Entry:  a.downloads[1],
			Status: "downloading",
			Done:   120,
			Total:  400,
		},
		jobKey(a.downloads[2]): {
			Entry:  a.downloads[2],
			Status: "downloaded",
		},
	}
	return a
}

// The view runs without a window in tests, which click as a user would.
func TestView(t *testing.T) {
	a := fixture()
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText("demo") {
		t.Fatalf("texts %q lack the project", tt.Texts())
	}
	if err := tt.Click(channelName(kindUnity) + " · " + locales.SidebarInstalls()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.InstallsTitle()) {
		t.Errorf("texts %q after choosing installs", tt.Texts())
	}
	if err := tt.Click(locales.SidebarSettings()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.SettingsLanguage()) {
		t.Errorf("texts %q after choosing settings", tt.Texts())
	}
	if err := tt.Click(channelName(kindUnity) + " · " + locales.SidebarDownloads()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.PageDownloads()) {
		t.Errorf("texts %q after choosing downloads", tt.Texts())
	}
	if err := tt.Click(locales.PageStore()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.LinkAssetStoreTuanjie()) {
		t.Errorf("texts %q after choosing the store", tt.Texts())
	}
	if err := tt.Click(locales.PageFeedback()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.Diagnostics()) {
		t.Errorf("texts %q after choosing feedback", tt.Texts())
	}
}

func TestFavoritesAndLearningPages(t *testing.T) {
	a := fixture()
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click(channelName(kindUnity) + " · " + locales.SidebarFavorites()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("demo") || tt.HasText("cozy") {
		t.Fatalf("favorites page: %q", tt.Texts())
	}
	if err := tt.Click(locales.PageLearn()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Unity Learn") {
		t.Fatalf("learning page: %q", tt.Texts())
	}
}

func TestEngineGroupsStaySeparateAndSorted(t *testing.T) {
	a := fixture()
	a.editors = append(a.editors,
		editorInstall{Version: "2022.3.62t2", Kind: kindTuanjie, Exe: "t2"},
		editorInstall{Version: "2022.3.62t17", Kind: kindTuanjie, Exe: "t17"},
	)
	a.downloads = append(a.downloads,
		downloadEntry{Engine: kindTuanjie, Version: "2022.3.62t2"},
		downloadEntry{Engine: kindUnity, Version: "6000.3.28b99"},
	)
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText("demo") || tt.HasText("cozy") {
		t.Fatalf("Unity projects: %q", tt.Texts())
	}
	if err := tt.Click(channelName(kindTuanjie) + " · " + locales.SidebarProjects()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("cozy") || tt.HasText("demo") {
		t.Fatalf("Tuanjie projects: %q", tt.Texts())
	}
	if err := tt.Click(channelName(kindTuanjie) + " · " + locales.SidebarInstalls()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2022.3.62t17") || tt.HasText("2022.3.51f1") {
		t.Fatalf("Tuanjie installs: %q", tt.Texts())
	}
	got := a.editorsForEngine(kindTuanjie)
	if len(got) != 2 || got[0].Version != "2022.3.62t17" || got[1].Version != "2022.3.62t2" {
		t.Fatalf("install order: %+v", got)
	}
	if err := tt.Click(channelName(kindTuanjie) + " · " + locales.SidebarDownloads()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2022.3.62t17") || tt.HasText("6000.3.28f1") {
		t.Fatalf("Tuanjie downloads: %q", tt.Texts())
	}
	gotDownloads := downloadsForEngine(a.downloads, kindUnity)
	if len(gotDownloads) != 3 || gotDownloads[0].Version != "6000.3.28f1" || gotDownloads[1].Version != "6000.3.28b99" {
		t.Fatalf("download order: %+v", gotDownloads)
	}
	if err := tt.Click(channelName(kindUnity) + " · " + locales.SidebarProjects()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("demo") || tt.HasText("cozy") {
		t.Fatalf("Unity projects after switching back: %q", tt.Texts())
	}
}

// SNAPSHOT=1 go test -run TestSnapshot draws the pages to PNG files, to look
// at them.
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT")
	if dir == "" {
		t.Skip("set SNAPSHOT to a directory to draw the pages")
	}
	a := fixture()
	for _, page := range []string{pageProjects, pageInstalls, pageDownloads, pageStore, pagePublish, pageLicense, pageDocs, pageFeedback, pageSettings} {
		a.page = page
		tt := ui.NewTester(a.view, 1180, 760)
		tt.Frame()
		name := filepath.Join(dir, "page"+page[1:]+".png")
		if err := writePNG(name, tt.Image()); err != nil {
			t.Fatal(err)
		}
		t.Log(name)
	}
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
