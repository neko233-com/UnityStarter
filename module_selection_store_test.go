package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func selectionFixture(t *testing.T) editorInstall {
	t.Helper()
	e := queueFixture()
	e.Exe = filepath.Join(t.TempDir(), "Editor", "Unity.exe")
	return e
}

func TestModuleSelectionStoreIsScopedAndValidated(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e := selectionFixture(t)
	other := selectionFixture(t)
	want := map[string]bool{"android": true, "webgl": true}
	if err := saveModuleSelection(e, want); err != nil {
		t.Fatal(err)
	}
	if err := saveModuleSelection(other, map[string]bool{"webgl": true}); err != nil {
		t.Fatal(err)
	}
	got, err := loadModuleSelection(e)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("selection=%v err=%v", got, err)
	}
	changedVersion := e
	changedVersion.Version = "7000.0.0a17"
	if got, err := loadModuleSelection(changedVersion); err != nil || len(got) != 0 {
		t.Fatalf("selection crossed versions: %v %v", got, err)
	}
	if err := saveModuleSelection(e, map[string]bool{"../unsafe": true}); err == nil {
		t.Fatal("invalid module ID saved")
	}
	if got, err := loadModuleSelection(e); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("invalid save changed existing selection")
	}
	if err := saveModuleSelection(e, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := loadModuleSelection(e); err != nil || len(got) != 0 {
		t.Fatalf("selection not cleared: %v %v", got, err)
	}
	if got, err := loadModuleSelection(other); err != nil || !got["webgl"] {
		t.Fatalf("other editor selection lost: %v %v", got, err)
	}
	path, err := moduleSelectionFile(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModuleSelection(e); err == nil {
		t.Fatal("corrupt selection silently accepted")
	}
}

func TestModuleSelectionRestoresAfterRestartWithoutInstalling(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e := selectionFixture(t)
	a := fixture()
	a.jobs = nil
	a.moduleSelectionStore, a.moduleSelectionLoad = saveModuleSelection, loadModuleSelection
	a.openEditorManagement(e)
	a.manageModules = true
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("Android Build Support"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Web Build Support"); err != nil {
		t.Fatal(err)
	}
	b := fixture()
	b.jobs = nil
	b.moduleSelectionStore, b.moduleSelectionLoad = saveModuleSelection, loadModuleSelection
	b.openEditorManagement(e)
	b.manageModules = true
	restored := ui.NewTester(b.view, 1180, 760)
	if len(b.moduleSelection) != 2 || len(b.moduleQueue) != 0 || b.moduleBusy || !restored.HasText(locales.ModuleSelectionRestored()) {
		t.Fatalf("unsafe restore: selection=%v queue=%v status=%q", b.moduleSelection, b.moduleQueue, b.moduleStatus)
	}
	var calls []string
	b.moduleQueueRunner = func(_ editorInstall, m editorModule, _ bool) {
		calls = append(calls, m.ID)
		if m.ID == "android" {
			b.moduleStatus = locales.InstallModuleDone()
			b.finishModuleQueueItem(m.ID, true)
		} else {
			b.moduleStatus = "fixture failure"
			b.finishModuleQueueItem(m.ID, false)
		}
	}
	if err := restored.Click(locales.ModuleBatchInstall(2)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"android", "webgl"}) {
		t.Fatalf("unexpected queue %v", calls)
	}
	remaining, err := loadModuleSelection(e)
	if err != nil || len(remaining) != 1 || !remaining["webgl"] {
		t.Fatalf("unfinished selection not saved: %v %v", remaining, err)
	}
}

func TestModuleQueueStopsIfSelectionCannotBeSaved(t *testing.T) {
	e := selectionFixture(t)
	a := newApp()
	a.manageEditor = e
	a.moduleSelection = map[string]bool{"android": true, "webgl": true}
	a.moduleQueue, a.moduleQueueTotal = []string{"android", "webgl"}, 2
	a.moduleSelectionStore = func(editorInstall, map[string]bool) error { return errors.New("disk unavailable") }
	started := false
	a.moduleQueueRunner = func(editorInstall, editorModule, bool) { started = true }
	a.finishModuleQueueItem("android", true)
	if started || len(a.moduleQueue) != 0 || a.moduleStatus != locales.ModuleSelectionSaveFailed() || !a.moduleSelection["webgl"] {
		t.Fatalf("queue continued after save failed: started=%v queue=%v status=%q", started, a.moduleQueue, a.moduleStatus)
	}
}

func TestModuleQueueDoesNotStartWhenInitialSelectionCannotBeSaved(t *testing.T) {
	e := selectionFixture(t)
	a := newApp()
	a.manageEditor = e
	a.moduleSelection = map[string]bool{"webgl": true}
	a.moduleSelectionStore = func(editorInstall, map[string]bool) error { return errors.New("disk unavailable") }
	started := false
	a.moduleQueueRunner = func(editorInstall, editorModule, bool) { started = true }
	a.startModuleQueue(e)
	if started || len(a.moduleQueue) != 0 || a.moduleStatus != locales.ModuleSelectionSaveFailed() {
		t.Fatalf("batch started without saved selection: started=%v queue=%v status=%q", started, a.moduleQueue, a.moduleStatus)
	}
}
