package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestDialogFootersStayInsideMinimumWindow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*app)
		footer string
	}{
		{"module-status", func(a *app) {
			a.manageEditor = queueFixture()
			a.manageOpen = true
			a.manageModules = true
			a.moduleStatus = strings.Repeat("Long status message. ", 20)
			a.moduleSelection = map[string]bool{"webgl": true}
			a.moduleSelectionRestored = true
		}, locales.ModuleBatchInstall(1)},
		{"module-progress", func(a *app) {
			a.manageEditor = queueFixture()
			a.manageOpen = true
			a.manageModules = true
			a.moduleBusy = true
			a.moduleQueue = []string{"webgl"}
			a.moduleQueueTotal = 3
			a.moduleStatus = strings.Repeat("Status ", 80)
			a.moduleProgressTotal = 100
		}, locales.ActionBackground()},
		{"android-license", func(a *app) {
			a.manageEditor = queueFixture()
			a.manageOpen = true
			a.manageModules = true
			a.androidPlan = &androidInstallPlan{License: strings.Repeat("License text\n", 80), CachedBytes: 120, Packages: make([]androidPackage, 8), LegacyTuanjie: true}
			a.moduleStatus = strings.Repeat("Status ", 40)
		}, locales.AndroidLicenseAccept()},
		{"alert", func(a *app) {
			a.alertOpen = true
			a.alertMessage = strings.Repeat("A long error message with details.\n", 60)
		}, locales.DialogOK()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newApp()
			tc.setup(a)
			tt := ui.NewTester(a.view, 900, 560)
			if dir := os.Getenv("LAYOUT_STRESS_SNAPSHOT"); dir != "" {
				_ = writePNG(filepath.Join(dir, tc.name+".png"), tt.Image())
			}
			r, ok := tt.Find(tc.footer)
			if !ok || r.W <= 0 || r.H < 12 || r.X < 0 || r.X+r.W > 900 || r.Y < 36 || r.Y+r.H > 540 {
				t.Fatalf("footer %q escaped window: %+v found=%v", tc.footer, r, ok)
			}
		})
	}
}

func TestBusyWorkspaceKeepsNavigationAndToolbarVisible(t *testing.T) {
	a := fixture()
	a.manageEditor = queueFixture()
	a.manageModules = true
	a.moduleBusy = true
	a.moduleStatus = locales.InstallModuleProgress()
	a.moduleProgressTotal = 100
	a.templateJob = &templateDownloadJob{Busy: true, Template: projectTemplate{Name: "URP"}}
	tt := ui.NewTester(a.view, 900, 560)
	for _, label := range []string{locales.PageLearn(), locales.PageFeedback(), locales.SidebarSettings(), locales.ActionNewProject()} {
		r, ok := tt.Find(label)
		if !ok || r.W <= 0 || r.H < 12 || r.X < 0 || r.X+r.W > 900 || r.Y < 36 || r.Y+r.H > 560 {
			t.Fatalf("control %q escaped viewport: %+v", label, r)
		}
		if label == locales.ActionNewProject() && r.Y > 180 {
			t.Fatalf("stacked tasks displaced toolbar: %+v", r)
		}
	}
	a.applyUpdateNow()
	if a.page != pageActivity || a.updateStatus != locales.UpdateInstallWait() {
		t.Fatal("template download did not keep updater on the native task page")
	}
}
