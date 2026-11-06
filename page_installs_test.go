package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func installedCardFixture() *app {
	a := fixture()
	a.activeEngine, a.page, a.jobs = kindTuanjie, pageTuanjieInstalls, nil
	modules := []editorModule{
		{ID: "android", Name: "Android Build Support", Selected: true, Visible: true},
		{ID: "ios", Name: "iOS Build Support", Selected: true, Visible: true},
		{ID: "windows-il2cpp", Name: "Windows Build Support (IL2CPP)", Selected: true, Visible: true},
		{ID: "weixinminigame", Name: "Mini Game Build Support", Selected: true, Visible: true},
		{ID: "documentation", Name: "Documentation", Selected: true, Visible: true},
		{ID: "android-sdk-ndk-tools", Name: "Android SDK & NDK Tools", Selected: true, Visible: true, Parent: "android"},
		{ID: "android-sdk-platform-tools", Name: "Android SDK Platform Tools", Selected: true, Visible: false},
		{ID: "android-sdk-build-tools", Name: "Android SDK Build Tools", Selected: true, Visible: false},
		{ID: "android-sdk-platforms-33", Name: "Android SDK Platforms 33", Selected: true, Visible: false},
		{ID: "android-sdk-platforms-34", Name: "Android SDK Platforms 34", Selected: true, Visible: false},
		{ID: "android-sdk-platforms-35", Name: "Android SDK Platforms 35", Selected: true, Visible: false},
		{ID: "android-ndk", Name: "Android NDK", Selected: true, Visible: false},
		{ID: "android-sdk-command-line-tools", Name: "Android SDK Command-Line Tools", Selected: true, Visible: false},
		{ID: "android-open-jdk", Name: "OpenJDK", Selected: true, Visible: true},
	}
	a.editors = []editorInstall{
		{Kind: kindTuanjie, Version: "2022.3.62t17", DisplayVersion: "1.11.0", Root: `D:\IDE\TuanJieEditor\2022.3.62t17`, Exe: `D:\IDE\TuanJieEditor\2022.3.62t17\Editor\Tuanjie.exe`, Modules: modules[:4]},
		{Kind: kindTuanjie, Version: "2022.3.62t15", DisplayVersion: "1.10.3", Root: `D:\IDE\TuanJieEditor\2022.3.62t15`, Exe: `D:\IDE\TuanJieEditor\2022.3.62t15\Editor\Tuanjie.exe`, Modules: modules},
	}
	return a
}

func TestInstalledCardShowsCompactModulesAndCompleteList(t *testing.T) {
	for _, width := range []int{900, 1400} {
		a := installedCardFixture()
		tt := ui.NewTester(a.view, width, 760)
		tt.SetTitleBar(ui.TitleBar{Height: 36, Right: 138})
		if !tt.HasText("Android") || !tt.HasText("iOS") || !tt.HasText("Windows") || !tt.HasText("+11") {
			t.Fatalf("compact module summary missing at %d: %q", width, tt.Texts())
		}
		for _, label := range []string{locales.InstallsLaunch(), locales.ActionFolder(), locales.InstallManage(), "+11"} {
			bounds, ok := tt.Find(label)
			if !ok || bounds.X < 288 || bounds.X+bounds.W > float32(width) {
				t.Fatalf("control %q outside card at %d: %+v, %v", label, width, bounds, ok)
			}
		}
		if err := tt.Click("+11"); err != nil {
			t.Fatal(err)
		}
		if !a.installedModulesOpen || !tt.HasText(locales.InstalledModulesTitle(14)) || !tt.HasText("Android SDK Platform Tools") {
			t.Fatalf("complete list or hidden dependency missing: %q", tt.Texts())
		}
		tt.Scroll(float32(width)/2, 350, 0, 2000)
		tt.Frame()
		if bounds, ok := tt.Find("OpenJDK"); !ok || bounds.Y < 100 || bounds.Y+bounds.H > 700 {
			t.Fatalf("last installed module is not reachable by scrolling: %+v, %v", bounds, ok)
		}
		if err := tt.Click(locales.InstallModules()); err != nil {
			t.Fatal(err)
		}
		if a.installedModulesOpen || !a.manageOpen || !a.manageModules || a.manageEditor.Version != "2022.3.62t15" {
			t.Fatal("could not open module management for the selected editor")
		}
	}
}

func TestInstalledCardSnapshot(t *testing.T) {
	dir := os.Getenv("INSTALLS_SNAPSHOT")
	if dir == "" {
		t.Skip("set INSTALLS_SNAPSHOT to a directory for visual inspection")
	}
	old := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return old }) })
	for _, size := range []struct{ width, height int }{{1400, 780}, {900, 560}} {
		a := installedCardFixture()
		if os.Getenv("INSTALLS_LIVE_MODULES") == "1" {
			for i := range a.editors {
				a.editors[i].Modules = readEditorModules(a.editors[i].Root)
			}
		}
		tt := ui.NewTester(a.view, size.width, size.height)
		tt.SetTitleBar(ui.TitleBar{Height: 36, Right: 138})
		if err := writePNG(filepath.Join(dir, fmt.Sprintf("installs-%d.png", size.width)), tt.Image()); err != nil {
			t.Fatal(err)
		}
		count := len(installedModuleNames(a.editors[1].Modules)) - len(installedModulesPreview(a.editors[1].Modules, 3))
		if count <= 0 {
			continue
		}
		if err := tt.Click(fmt.Sprintf("+%d", count)); err != nil {
			t.Fatal(err)
		}
		if err := writePNG(filepath.Join(dir, fmt.Sprintf("modules-%d.png", size.width)), tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}
