package main

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

func TestDownloadErrorsAreSeparatedByEngine(t *testing.T) {
	a := fixture()
	a.downloadsErrs = map[string]string{kindUnity: "unity unavailable", kindTuanjie: "tuanjie unavailable"}
	a.page = pageDownloads
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.DownloadsError("unity unavailable")) || tt.HasText(locales.DownloadsError("tuanjie unavailable")) {
		t.Fatalf("Unity errors: %q", tt.Texts())
	}
	a.page = pageTuanjieDownloads
	tt.Frame()
	if !tt.HasText(locales.DownloadsError("tuanjie unavailable")) || tt.HasText(locales.DownloadsError("unity unavailable")) {
		t.Fatalf("Tuanjie errors: %q", tt.Texts())
	}
}

func TestDownloadCatalogUpdatesEachEngineIndependently(t *testing.T) {
	a := fixture()
	a.downloadsPending = map[string]bool{kindUnity: true, kindTuanjie: true}
	a.applyReleaseCatalog(releaseCatalogResult{engine: kindTuanjie, entries: []downloadEntry{{Engine: kindTuanjie, Version: "2022.3.62t17"}}})
	if len(downloadsForEngine(a.downloads, kindTuanjie)) != 1 || len(downloadsForEngine(a.downloads, kindUnity)) != 2 || a.downloadsPending[kindTuanjie] || !a.downloadsPending[kindUnity] {
		t.Fatalf("first catalog lost independent state: %+v pending=%v", a.downloads, a.downloadsPending)
	}
}

func TestDownloadCatalogKeepsCurrentEntriesWhenRefreshFails(t *testing.T) {
	a := fixture()
	a.downloadsPending = map[string]bool{kindUnity: true}
	a.applyReleaseCatalog(releaseCatalogResult{engine: kindUnity, err: errors.New("offline")})
	unity := downloadsForEngine(a.downloads, kindUnity)
	if len(unity) != 2 || a.downloadsErrs[kindUnity] == "" || a.downloadsPending[kindUnity] {
		t.Fatalf("failed refresh lost current releases or its error: %+v %v", unity, a.downloadsErrs)
	}
	if len(downloadsForEngine(a.downloads, kindTuanjie)) != 1 {
		t.Fatal("failed Unity refresh changed Tuanjie releases")
	}
	a.applyReleaseCatalog(releaseCatalogResult{engine: kindUnity, entries: []downloadEntry{{Engine: kindUnity, Version: "7000.0.0a16"}}})
	unity = downloadsForEngine(a.downloads, kindUnity)
	if len(unity) != 1 || unity[0].Version != "7000.0.0a16" || a.downloadsErrs[kindUnity] != "" {
		t.Fatalf("successful refresh did not replace stale releases: %+v %v", unity, a.downloadsErrs)
	}
	a.downloads = nil
	a.applyReleaseCatalog(releaseCatalogResult{engine: kindUnity, entries: []downloadEntry{{Engine: kindUnity, Version: "cached"}}, err: errors.New("offline")})
	if len(a.downloads) != 1 || a.downloads[0].Version != "cached" {
		t.Fatalf("initial offline cache was not used: %+v", a.downloads)
	}
}

func TestReleaseSearchKeepsEnginesAndRequestsSeparate(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	a.downloadQueryUnity = "2022.3"
	a.applyUnityReleaseSearch("old query", []downloadEntry{{Engine: kindUnity, Version: "old"}}, nil)
	if a.unitySearchQuery != "" {
		t.Fatal("stale search replaced current query")
	}
	a.applyUnityReleaseSearch("2022.3", []downloadEntry{
		{Engine: kindUnity, Version: "2022.3.9f1"},
		{Engine: kindUnity, Version: "2022.3.62f1"},
	}, nil)
	entries := a.visibleDownloads(kindUnity)
	if len(entries) != 2 || entries[0].Version != "2022.3.62f1" {
		t.Fatalf("archive search order: %+v", entries)
	}
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.DownloadsSearchOfficial()) || !tt.HasText("2022.3.62f1") || !tt.HasText(locales.DownloadsSearchLimit()) {
		t.Fatalf("archive search controls or results missing: %q", tt.Texts())
	}
	if path := os.Getenv("DOWNLOAD_SEARCH_SNAPSHOT"); path != "" {
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	a.downloadQueryTuanjie = "62t17"
	tuanjie := a.visibleDownloads(kindTuanjie)
	if len(tuanjie) != 1 || tuanjie[0].Engine != kindTuanjie {
		t.Fatalf("Tuanjie query mixed with Unity search: %+v", tuanjie)
	}
	a.downloadQueryUnity = "6000"
	a.applyUnityReleaseSearch("6000", nil, errors.New("offline"))
	if got := a.visibleDownloads(kindUnity); len(got) != 2 || a.unitySearchErr == "" {
		t.Fatalf("offline search lost local matches: %+v error=%q", got, a.unitySearchErr)
	}
}

func TestReleaseSearchInputFiltersTheCurrentEngine(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click(locales.DownloadsSearchHint()); err != nil {
		t.Fatal(err)
	}
	tt.Type("6000.0")
	if a.downloadQueryUnity != "6000.0" || len(a.visibleDownloads(kindUnity)) != 1 {
		t.Fatalf("search input not applied: %q %+v", a.downloadQueryUnity, a.visibleDownloads(kindUnity))
	}
	a.page = pageTuanjieDownloads
	tt.Frame()
	if a.downloadQueryTuanjie != "" || len(a.visibleDownloads(kindTuanjie)) != 1 {
		t.Fatal("Unity query leaked into Tuanjie")
	}
}

func TestArchiveDownloadRemainsVisibleAfterSearchCleared(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	historical := downloadEntry{Engine: kindUnity, Version: "2022.3.62f1", URL: "https://download.unity3d.com/old.exe"}
	a.jobs[jobKey(historical)] = &downloadJob{Entry: historical, Status: "downloading", Done: 10, Total: 100}
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.DownloadsActive()) || !tt.HasText(historical.Version) {
		t.Fatalf("active historical download disappeared: %q", tt.Texts())
	}
	a.page = pageTuanjieDownloads
	tt.Frame()
	if tt.HasText(historical.Version) {
		t.Fatal("Unity download appeared in Tuanjie group")
	}
}

func TestAndroidLicenseCanBeReviewedAndCanceled(t *testing.T) {
	a := fixture()
	a.manageOpen, a.manageModules = true, true
	a.manageEditor = editorInstall{Version: "7000.0.0a16", Kind: kindUnity}
	a.androidPlan = &androidInstallPlan{License: "Terms fixture for review", CachedBytes: 256 << 20, Packages: []androidPackage{{}, {}}}
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.AndroidLicenseTitle()) || !tt.HasText("Terms fixture for review") || !tt.HasText(locales.AndroidLicenseAccept()) {
		t.Fatalf("license dialog missing: %q", tt.Texts())
	}
	if !tt.HasText(locales.AndroidCachedBytes("256.0 MB")) || !tt.HasText(locales.AndroidDiscardCache()) {
		t.Fatalf("cached package controls missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.androidPlan != nil || a.moduleBusy {
		t.Fatal("cancel should not start installation")
	}
}

func TestModuleCatalogOfflineNoticeAndRefresh(t *testing.T) {
	a := fixture()
	a.manageOpen, a.manageModules = true, true
	a.manageEditor = editorInstall{Version: "7000.0.0a16", Kind: kindUnity, ModulesCached: true, Modules: []editorModule{{ID: "webgl", Name: "Web", Visible: true, Selected: true}}}
	a.moduleStatus = locales.InstallModuleCached()
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.InstallModuleCached()) || !tt.HasText(locales.InstallModuleRefresh()) || !tt.HasText("Web") || !tt.HasText(locales.DownloadsInstalled()) {
		t.Fatalf("offline module management: %q", tt.Texts())
	}
	if path := os.Getenv("MODULE_CATALOG_SNAPSHOT"); path != "" {
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOfflineDocumentationFollowsActiveEngine(t *testing.T) {
	a := fixture()
	a.page, a.activeEngine = pageDocs, kindTuanjie
	a.editors = append(a.editors, editorInstall{Kind: kindTuanjie, Version: "2022.3.62t15", Exe: "tuanjie-editor", OfflineDocs: `D:\Docs\Tuanjie`})
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.OfflineDocsTitle()) || !tt.HasText(locales.OfflineDocsManual()) || !tt.HasText(locales.OfflineDocsAPI()) || !tt.HasText("2022.3.62t15") {
		t.Fatalf("offline documentation missing: %q", tt.Texts())
	}
	if path := os.Getenv("OFFLINE_DOCS_SNAPSHOT"); path != "" {
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	a.activeEngine = kindUnity
	tt.Frame()
	if tt.HasText(locales.OfflineDocsTitle()) {
		t.Fatal("Tuanjie offline documentation appeared in Unity")
	}
}

func TestAndroidRecoveryStillRequiresLicenseConfirmation(t *testing.T) {
	a := fixture()
	a.manageOpen, a.manageModules = true, true
	a.manageEditor = editorInstall{Version: "7000.0.0a16", Kind: kindUnity}
	a.androidPlan = &androidInstallPlan{License: "Terms fixture for recovery", CachedBytes: 256 << 20, RecoveryPending: true}
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.AndroidRecoveryPending()) || !tt.HasText(locales.AndroidLicenseRecover()) {
		t.Fatalf("recovery confirmation missing: %q", tt.Texts())
	}
	if tt.HasText(locales.AndroidDiscardCache()) {
		t.Fatal("recovery journal must not be discarded from the consent screen")
	}
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if a.androidPlan != nil || a.moduleBusy {
		t.Fatal("canceling recovery must not start installation")
	}
}

func TestAndroidLicenseSnapshot(t *testing.T) {
	path := os.Getenv("ANDROID_LICENSE_SNAPSHOT")
	if path == "" {
		t.Skip("set ANDROID_LICENSE_SNAPSHOT to inspect the native dialog")
	}
	a := fixture()
	a.manageOpen, a.manageModules = true, true
	a.manageEditor = editorInstall{Version: "7000.0.0a16", Kind: kindUnity}
	a.androidPlan = &androidInstallPlan{License: strings.Repeat("Terms fixture. This text represents a long agreement to review before installing.\n\n", 200) + "END OF LICENSE FIXTURE", CachedBytes: 256 << 20, Packages: make([]androidPackage, 9)}
	for i := range a.androidPlan.Packages {
		a.androidPlan.Packages[i].ExpectedSize = 120000000
	}
	tt := ui.NewTester(a.view, 1180, 760)
	tt.Frame()
	if err := writePNG(path, tt.Image()); err != nil {
		t.Fatal(err)
	}
	tt.Scroll(550, 400, 0, 200000)
	tt.Frame()
	if err := writePNG(path+".bottom.png", tt.Image()); err != nil {
		t.Fatal(err)
	}
	small := ui.NewTester(a.view, 900, 560)
	small.Frame()
	if err := writePNG(path+".small.png", small.Image()); err != nil {
		t.Fatal(err)
	}
	a.androidPlan.RecoveryPending = true
	recovery := ui.NewTester(a.view, 900, 560)
	recovery.Frame()
	if err := writePNG(path+".recovery.png", recovery.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadFailureCanDiscardSavedProgress(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	e := a.downloads[0]
	dir := t.TempDir()
	dest := filepath.Join(dir, "editor.exe")
	file, _, _, _, release, err := openResume(dest, e.URL, "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", `"stable"`, parallelMinimumSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	a.jobs[jobKey(e)] = &downloadJob{Entry: e, Path: dest, Status: "failedDownload", Err: "network interrupted"}
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.ActionRetryDownload()) || !tt.HasText(locales.ActionDiscardDownload()) {
		t.Fatalf("download recovery controls missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.ActionDiscardDownload()); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.jobs[jobKey(e)]; ok {
		t.Fatal("failed job not cleared")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("progress not removed: %v %v", entries, err)
	}
}

func TestDownloadRecoverySnapshot(t *testing.T) {
	path := os.Getenv("DOWNLOAD_RECOVERY_SNAPSHOT")
	if path == "" {
		t.Skip("set DOWNLOAD_RECOVERY_SNAPSHOT to inspect download controls")
	}
	a := fixture()
	a.page = pageDownloads
	e := a.downloads[0]
	a.jobs[jobKey(e)] = &downloadJob{Entry: e, Path: filepath.Join(t.TempDir(), "editor.exe"), Status: "failedDownload", Err: "network interrupted; resume available"}
	a.jobs[jobKey(a.downloads[1])].Speed = 15 << 20
	tt := ui.NewTester(a.view, 1180, 760)
	tt.Frame()
	if err := writePNG(path, tt.Image()); err != nil {
		t.Fatal(err)
	}
	small := ui.NewTester(a.view, 900, 560)
	small.Frame()
	if err := writePNG(path+".small.png", small.Image()); err != nil {
		t.Fatal(err)
	}
}

// The view runs without a window in tests, which click as a user would.
func TestView(t *testing.T) {
	a := fixture()
	tt := ui.NewTester(a.view, 1180, 760)
	tt.SetTitleBar(ui.TitleBar{Height: 36, Right: 138})
	if title, ok := tt.Find(locales.AppTitle()); !ok || title.Y < 0 || title.Y >= 36 {
		t.Fatalf("title missing from native title bar: %+v, %v", title, ok)
	}
	if !tt.HasText("demo") {
		t.Fatalf("texts %q lack the project", tt.Texts())
	}
	if err := tt.Click(locales.SidebarInstalls()); err != nil {
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
	if err := tt.Click(locales.SidebarDownloads()); err != nil {
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

func TestNativeAlertAndUpdateDialogs(t *testing.T) {
	a := fixture()
	a.alert("a specific problem")
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText("a specific problem") {
		t.Fatalf("alert missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.DialogOK()); err != nil {
		t.Fatal(err)
	}
	if a.alertOpen {
		t.Fatal("alert did not close")
	}
	a.availableVersion, a.availableOpen = "9.0.0", true
	tt.Frame()
	if !tt.HasText(locales.UpdateAvailable("9.0.0")) {
		t.Fatalf("update prompt missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if a.availableOpen {
		t.Fatal("update dialog did not close")
	}
}

func TestFavoritesAndLearningPages(t *testing.T) {
	a := fixture()
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click(locales.SidebarFavorites()); err != nil {
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
	if err := tt.Click("T"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("cozy") || tt.HasText("demo") {
		t.Fatalf("Tuanjie projects: %q", tt.Texts())
	}
	if err := tt.Click(locales.SidebarInstalls()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2022.3.62t17") || tt.HasText("2022.3.51f1") {
		t.Fatalf("Tuanjie installs: %q", tt.Texts())
	}
	got := a.editorsForEngine(kindTuanjie)
	if len(got) != 2 || got[0].Version != "2022.3.62t17" || got[1].Version != "2022.3.62t2" {
		t.Fatalf("install order: %+v", got)
	}
	if err := tt.Click(locales.SidebarDownloads()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2022.3.62t17") || tt.HasText("6000.3.28f1") {
		t.Fatalf("Tuanjie downloads: %q", tt.Texts())
	}
	gotDownloads := downloadsForEngine(a.downloads, kindUnity)
	if len(gotDownloads) != 3 || gotDownloads[0].Version != "6000.3.28f1" || gotDownloads[1].Version != "6000.3.28b99" {
		t.Fatalf("download order: %+v", gotDownloads)
	}
	if err := tt.Click("U"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.SidebarProjects()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("demo") || tt.HasText("cozy") {
		t.Fatalf("Unity projects after switching back: %q", tt.Texts())
	}
}

func TestProjectSelectVersionAndConfirmBeforeLaunch(t *testing.T) {
	a := fixture()
	path := t.TempDir()
	settings := filepath.Join(path, "ProjectSettings")
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	versionFile := filepath.Join(settings, "ProjectVersion.txt")
	original := []byte("m_EditorVersion: 2022.3.51f1\n")
	if err := os.WriteFile(versionFile, original, 0o644); err != nil {
		t.Fatal(err)
	}
	a.projects[0].Path = path
	a.editors = append(a.editors,
		editorInstall{Version: "6000.0.1f1", Kind: kindUnity, Exe: "chosen-editor"},
		editorInstall{Version: "2022.3.62t17", Kind: kindTuanjie, Exe: "wrong-engine"},
	)
	var launched string
	var args []string
	a.launchEditor = func(exe string, argv []string, _ string) error { launched = exe; args = argv; return nil }
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("2022.3.51f1 ▾"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("6000.0.1f1") || tt.HasText("2022.3.62t17") {
		t.Fatalf("version choices: %q", tt.Texts())
	}
	if err := tt.Click(locales.ProjectVersionOpen()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.ProjectVersionWarning()) || launched != "" {
		t.Fatalf("missing confirmation: %q", tt.Texts())
	}
	if err := tt.Click(locales.ProjectVersionContinue()); err != nil {
		t.Fatal(err)
	}
	if launched != "chosen-editor" || len(args) != 2 || args[0] != "-projectPath" || args[1] != path {
		t.Fatalf("launch: %q %q", launched, args)
	}
	if got, err := os.ReadFile(versionFile); err != nil || string(got) != string(original) {
		t.Fatalf("project version changed before editor launch: %q %v", got, err)
	}
}

func TestProjectSortAndMissingVersionChoice(t *testing.T) {
	a := fixture()
	a.projects = append(a.projects, projectEntry{Name: "alpha", Path: "alpha", Version: "6000.0.2f1", Kind: kindUnity})
	a.sortProjects("name")
	got := a.filteredProjects()
	if len(got) != 2 || got[0].Name != "alpha" {
		t.Fatalf("name sort: %+v", got)
	}
	a.sortProjects("name")
	if got = a.filteredProjects(); got[0].Name != "demo" {
		t.Fatalf("descending sort: %+v", got)
	}
	a.chooseProjectVersion(a.projects[1])
	if !a.versionOpen || a.versionEditor != "" {
		t.Fatal("simulated editor must not be selected")
	}
	ui.NewTester(a.view, 1180, 760).Frame()
}

func TestNewProjectEditorSelectionPersistsAcrossFrames(t *testing.T) {
	a := fixture()
	a.editors = []editorInstall{
		{Version: "6000.0.2f1", Kind: kindUnity, Exe: `C:\Editors\new\Unity.exe`},
		{Version: "2022.3.51f1", Kind: kindUnity, Exe: `C:\Editors\old\Unity.exe`},
	}
	a.openNewProject()
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("6000.0.2f1 · " + channelName(kindUnity)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("2022.3.51f1 · " + channelName(kindUnity)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if a.newEditor != `C:\Editors\old\Unity.exe` {
		t.Fatalf("selected editor was lost: %q", a.newEditor)
	}
}

func TestNewProjectCanChooseBetweenSameVersionInstallations(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	a := fixture()
	a.editors = []editorInstall{
		{Version: "6000.0.2f1", Kind: kindUnity, Exe: `C:\Editors\one\Unity.exe`},
		{Version: "6000.0.2f1", Kind: kindUnity, Exe: `D:\Editors\two\Unity.exe`},
	}
	a.openNewProject()
	a.newDir, a.newName, a.prefs.NoHubSync = t.TempDir(), "Game", true
	var launched string
	var args []string
	a.launchEditor = func(exe string, argv []string, _ string) error { launched, args = exe, argv; return nil }
	labels, _ := a.editorLabels()
	if len(labels) != 2 || labels[0] == labels[1] {
		t.Fatalf("same-version editors need distinct labels: %q", labels)
	}
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click(labels[0]); err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("NEW_PROJECT_PICKER_SNAPSHOT"); path != "" {
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click(labels[1]); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.newEditor != `D:\Editors\two\Unity.exe` {
		t.Fatalf("wrong installation selected: %q", a.newEditor)
	}
	if err := tt.Click(locales.ActionCreate()); err != nil {
		t.Fatal(err)
	}
	if launched != a.newEditor || len(args) != 2 || args[0] != "-createProject" || args[1] != filepath.Join(a.newDir, "Game") {
		t.Fatalf("wrong editor or project path passed to launch: %q %q", launched, args)
	}
}

func TestNewProjectRejectsNamesThatEscapeOrAreInvalid(t *testing.T) {
	for _, name := range []string{"..", `..\outside`, "../outside", `C:\outside`, "CON", "game?"} {
		t.Run(name, func(t *testing.T) {
			a := fixture()
			a.newDir, a.newName, a.newEditor, a.newEngine = t.TempDir(), name, "chosen", kindUnity
			a.editors = []editorInstall{{Exe: "chosen", Kind: kindUnity}}
			launched := false
			a.launchEditor = func(string, []string, string) error { launched = true; return nil }
			a.createProject()
			if launched || a.newError != locales.NewProjectInvalidName() {
				t.Fatalf("invalid name accepted: launched=%v error=%q", launched, a.newError)
			}
			entries, err := os.ReadDir(a.newDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid name changed destination: %v %v", entries, err)
			}
		})
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
