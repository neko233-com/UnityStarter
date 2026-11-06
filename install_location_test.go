package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestChangingInstallLocationKeepsExistingEditors(t *testing.T) {
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	if _, err := simulateEditorInstall(kindUnity, "6000.0.1f1", oldRoot); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.prefs.InstallRoot = oldRoot
	a.setInstallRoot(newRoot)
	if a.installRoot() != newRoot {
		t.Fatalf("new install location not applied: %q", a.installRoot())
	}
	editors := scanEditorsIn(append([]string{a.installRoot()}, a.prefs.EditorRoots...))
	if len(editors) != 1 || editors[0].Version != "6000.0.1f1" {
		t.Fatalf("editor in previous location disappeared: %+v", editors)
	}
	if _, err := simulateEditorInstall(kindUnity, "6000.0.2f1", a.installRoot()); err != nil {
		t.Fatal(err)
	}
	editors = scanEditorsIn(append([]string{a.installRoot()}, a.prefs.EditorRoots...))
	if len(editors) != 2 {
		t.Fatalf("editors in both locations should remain visible: %+v", editors)
	}
}

func TestInstallLocationShownOnInstallPages(t *testing.T) {
	a := fixture()
	a.prefs.InstallRoot = `D:\Editors`
	a.page = pageInstalls
	tt := ui.NewTester(a.view, 900, 560)
	for _, page := range []string{pageInstalls, pageTuanjieInstalls} {
		a.page = page
		tt.Frame()
		if !tt.HasText(locales.InstallChooseLocation()) || !tt.HasText(a.prefs.InstallRoot) || !tt.HasText(locales.ActionAddFolder()) {
			t.Fatalf("install location or existing-editor action missing: %q", tt.Texts())
		}
		if path := os.Getenv("INSTALL_LOCATION_SNAPSHOT"); path != "" && page == pageInstalls {
			if err := writePNG(path, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
	}
}
