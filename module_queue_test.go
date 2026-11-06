package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func queueFixture() editorInstall {
	android := editorModule{ID: "android", Name: "Android Build Support", Visible: true, URL: "https://download.unity3d.com/android.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
	_, sdk := androidFixture()
	sdk.Name, sdk.Visible = "Android SDK and NDK", true
	jdk := jdkFixtureModule()
	jdk.Name, jdk.Visible = "OpenJDK", true
	web := editorModule{ID: "webgl", Name: "Web Build Support", Visible: true, URL: "https://download.unity3d.com/web.exe", Checksum: strings.Repeat("b", 32), Destination: "{UNITY_PATH}"}
	return editorInstall{Kind: kindUnity, Version: "7000.0.0a16", Exe: "editor", Modules: []editorModule{sdk, jdk, android, web}}
}

func TestModuleQueuePlansDependenciesFirst(t *testing.T) {
	e := queueFixture()
	selection := map[string]bool{"android": true, "android-sdk-ndk-tools": true, "android-open-jdk-17.0.18+8": true, "webgl": true}
	got, err := planModuleQueue(e, selection)
	want := []string{"android", "android-sdk-ndk-tools", "android-open-jdk-17.0.18+8", "webgl"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("plan=%v err=%v", got, err)
	}
	delete(selection, "android")
	if _, err := planModuleQueue(e, selection); err == nil {
		t.Fatal("missing Android dependency accepted")
	}
	e.Modules[2].Selected = true
	if got, err := planModuleQueue(e, selection); err != nil || len(got) != 3 {
		t.Fatalf("installed dependency rejected: %v %v", got, err)
	}
}

func TestModuleQueueRejectsStaleUnsupportedOrAmbiguousSelection(t *testing.T) {
	for _, mode := range []string{"missing", "unsupported", "duplicate", "simulated"} {
		t.Run(mode, func(t *testing.T) {
			e := queueFixture()
			selection := map[string]bool{"webgl": true}
			switch mode {
			case "missing":
				selection["deleted"] = true
			case "unsupported":
				e.Modules[3].URL = "https://example.invalid/web.exe"
			case "duplicate":
				e.Modules = append(e.Modules, e.Modules[3])
			case "simulated":
				e.Simulated = true
			}
			if _, err := planModuleQueue(e, selection); err == nil {
				t.Fatal("unsafe selection accepted")
			}
		})
	}
}

func TestRemovingParentClearsSelectedDependencies(t *testing.T) {
	e := queueFixture()
	a := newApp()
	a.moduleSelection = map[string]bool{"android": true, "android-sdk-ndk-tools": true, "android-open-jdk-17.0.18+8": true, "webgl": true}
	a.setModuleSelection(e, "android", false)
	if len(a.moduleSelection) != 1 || !a.moduleSelection["webgl"] {
		t.Fatalf("dependent selection not cleared: %v", a.moduleSelection)
	}
}

func TestModuleSelectionReconcilesCatalogChanges(t *testing.T) {
	e := queueFixture()
	a := newApp()
	a.moduleSelection = map[string]bool{"android": true, "android-sdk-ndk-tools": true, "webgl": true, "removed": true}
	e.Modules[2].Selected = true
	if !a.reconcileModuleSelection(e) || len(a.moduleSelection) != 2 || !a.moduleSelection["android-sdk-ndk-tools"] || !a.moduleSelection["webgl"] {
		t.Fatalf("changed catalog not reconciled: %v", a.moduleSelection)
	}
	e.Modules = e.Modules[:2] // Android parent and web are no longer available.
	if !a.reconcileModuleSelection(e) || len(a.moduleSelection) != 0 {
		t.Fatalf("missing dependencies retained: %v", a.moduleSelection)
	}
}

func TestModuleQueueStopsOnFailureAndRetainsUnfinishedSelection(t *testing.T) {
	e := queueFixture()
	a := newApp()
	a.manageEditor = e
	a.moduleSelection = map[string]bool{"android": true, "webgl": true}
	a.moduleQueue = []string{"android", "webgl"}
	a.moduleQueueTotal = 2
	a.moduleStatus = locales.InstallModuleDone()
	a.finishModuleQueueItem("android", true)
	if len(a.moduleQueue) != 1 || a.moduleQueue[0] != "webgl" || a.moduleSelection["android"] || !a.moduleSelection["webgl"] {
		t.Fatalf("wrong queue after success: %v %v", a.moduleQueue, a.moduleSelection)
	}
	a.moduleStatus = "network failure"
	a.finishModuleQueueItem("webgl", false)
	if len(a.moduleQueue) != 0 || !a.moduleSelection["webgl"] || a.moduleStatus != "network failure" {
		t.Fatalf("failure lost recovery state: %v %v %q", a.moduleQueue, a.moduleSelection, a.moduleStatus)
	}
}

func TestModuleQueueRunsDependenciesAndWaitsForAndroidConsent(t *testing.T) {
	e := queueFixture()
	selection := map[string]bool{"android": true, "android-sdk-ndk-tools": true, "webgl": true}
	queue, err := planModuleQueue(e, selection)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.manageEditor, a.moduleSelection, a.moduleQueue, a.moduleQueueTotal = e, selection, queue, len(queue)
	var started []string
	waiting := false
	a.moduleQueueRunner = func(_ editorInstall, m editorModule, android bool) {
		started = append(started, m.ID)
		if android {
			waiting = true
			return
		}
		for i := range a.manageEditor.Modules {
			if a.manageEditor.Modules[i].ID == m.ID {
				a.manageEditor.Modules[i].Selected = true
			}
		}
		a.moduleStatus = locales.InstallModuleDone()
		a.finishModuleQueueItem(m.ID, true)
	}
	a.advanceModuleQueue()
	if !waiting || !reflect.DeepEqual(started, []string{"android", "android-sdk-ndk-tools"}) || len(a.moduleQueue) != 2 || !a.moduleSelection["android-sdk-ndk-tools"] {
		t.Fatalf("queue did not wait for consent: started=%v queue=%v selection=%v", started, a.moduleQueue, a.moduleSelection)
	}
	// Represents the explicitly accepted Android installation succeeding.
	a.moduleStatus = locales.InstallModuleDone()
	a.finishModuleQueueItem("android-sdk-ndk-tools", true)
	if !reflect.DeepEqual(started, []string{"android", "android-sdk-ndk-tools", "webgl"}) || len(a.moduleQueue) != 0 || len(a.moduleSelection) != 0 || a.moduleStatus != locales.ModuleBatchDone() {
		t.Fatalf("queue did not finish: started=%v queue=%v selection=%v status=%q", started, a.moduleQueue, a.moduleSelection, a.moduleStatus)
	}
}

func TestModuleBatchSelectionInNativeDialog(t *testing.T) {
	e := queueFixture()
	a := fixture()
	a.manageEditor, a.manageOpen, a.manageModules = e, true, true
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("Android Build Support"); err != nil {
		t.Fatal(err)
	}
	if !a.moduleSelection["android"] {
		t.Fatal("parent not selected")
	}
	if err := tt.Click("Android SDK and NDK"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Web Build Support"); err != nil {
		t.Fatal(err)
	}
	if len(a.moduleSelection) != 3 || !tt.HasText(locales.ModuleBatchInstall(3)) {
		t.Fatalf("selection controls: %v texts=%v", a.moduleSelection, tt.Texts())
	}
	if path := os.Getenv("MODULE_BATCH_SNAPSHOT"); path != "" {
		tt.Frame()
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("Android Build Support"); err != nil {
		t.Fatal(err)
	}
	if len(a.moduleSelection) != 1 || !a.moduleSelection["webgl"] {
		t.Fatalf("child survived parent deselection: %v", a.moduleSelection)
	}
}

func TestModuleBatchLicenseCancellationKeepsRemainingSelection(t *testing.T) {
	a := fixture()
	e, sdk := androidFixture()
	a.manageEditor, a.manageOpen, a.manageModules = e, true, true
	a.moduleSelection = map[string]bool{sdk.ID: true, "webgl": true}
	a.moduleQueue, a.moduleQueueTotal = []string{sdk.ID, "webgl"}, 2
	a.moduleQueueWarnings = []string{"earlier module warning"}
	a.androidPlan = &androidInstallPlan{Module: sdk, License: "Terms fixture"}
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if a.androidPlan != nil || len(a.moduleQueue) != 0 || len(a.moduleSelection) != 2 || !strings.Contains(a.moduleStatus, locales.ModuleBatchStopped()) || !strings.Contains(a.moduleStatus, "earlier module warning") {
		t.Fatalf("cancel did not stop safely: queue=%v selection=%v status=%q", a.moduleQueue, a.moduleSelection, a.moduleStatus)
	}
}

func TestPendingAndroidConsentCanBeReopened(t *testing.T) {
	a := fixture()
	a.jobs = nil
	e, sdk := androidFixture()
	e.Exe = "editor"
	a.manageEditor, a.manageOpen, a.manageModules = e, false, true
	a.androidPlan = &androidInstallPlan{Module: sdk, License: "Terms fixture"}
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.AndroidLicenseTitle()) || !tt.HasText(locales.InstallModules()) || !a.editorWorkActive() {
		t.Fatalf("pending consent could not be recovered: %v", tt.Texts())
	}
	other := editorInstall{Exe: "another"}
	a.openEditorManagement(other)
	if a.manageEditor.Exe != "editor" || a.androidPlan == nil {
		t.Fatal("pending consent was replaced by another editor")
	}
	a.openEditorManagement(e)
	if !a.manageOpen || a.androidPlan == nil {
		t.Fatal("pending consent was not reopened")
	}
}

func TestModuleBatchButtonRunsSelectedQueue(t *testing.T) {
	e := queueFixture()
	a := fixture()
	a.jobs = nil
	a.manageEditor, a.manageOpen, a.manageModules = e, true, true
	var started []string
	a.moduleQueueRunner = func(_ editorInstall, m editorModule, android bool) {
		if android {
			t.Fatal("unexpected Android SDK consent")
		}
		started = append(started, m.ID)
		for i := range a.manageEditor.Modules {
			if a.manageEditor.Modules[i].ID == m.ID {
				a.manageEditor.Modules[i].Selected = true
			}
		}
		a.moduleStatus = locales.InstallModuleDone()
		a.finishModuleQueueItem(m.ID, true)
	}
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("Android Build Support"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Web Build Support"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.ModuleBatchInstall(2)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started, []string{"android", "webgl"}) || len(a.moduleSelection) != 0 || a.moduleStatus != locales.ModuleBatchDone() {
		t.Fatalf("batch click: started=%v selection=%v status=%q", started, a.moduleSelection, a.moduleStatus)
	}
}
