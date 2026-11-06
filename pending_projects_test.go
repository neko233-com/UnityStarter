package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestProjectCreationRegistersOnlyAfterFilesExist(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "New Game")
	e := editorInstall{Version: "6000.0.2f1", Kind: kindUnity, Exe: "editor"}
	h := hub{Kind: kindUnity, Registry: filepath.Join(t.TempDir(), "projects-v1.json")}
	launched := false
	if err := startProjectCreation(path, e, func(_ string, args []string, _ string) error {
		launched = len(args) == 2 && args[0] == "-createProject" && args[1] == path
		return nil
	}); err != nil || !launched {
		t.Fatalf("start: launched=%v error=%v", launched, err)
	}
	pending := pendingProjects()
	if len(pending) != 1 || pendingProjectReady(pending[0]) || h.lists(path) {
		t.Fatalf("project incorrectly registered before creation: %+v", pending)
	}
	writeProject(t, path, e.Version)
	if pendingProjectReady(pending[0]) {
		t.Fatal("version file alone marked project ready")
	}
	if err := os.MkdirAll(filepath.Join(path, "Assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "Packages"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(path, "Packages", "manifest.json")
	if err := os.WriteFile(manifest, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if pendingProjectReady(pending[0]) {
		t.Fatal("partial manifest accepted")
	}
	if err := os.WriteFile(manifest, []byte(`{"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate restarting the application before the project became ready.
	a := newApp()
	a.hubs = []hub{h}
	ready := pendingProjects()
	if len(ready) != 1 || !pendingProjectReady(ready[0]) {
		t.Fatal("ready creation was not recovered")
	}
	a.finishPendingProjects(ready)
	if !h.lists(path) || len(pendingProjects()) != 0 || len(loadPrefs().PinnedProjects) != 1 {
		t.Fatalf("project was not registered and pinned: %+v", loadPrefs())
	}
}

func TestProjectCreationLaunchFailureRemovesOnlyIntent(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "Game")
	e := editorInstall{Version: "6000.0.2f1", Kind: kindUnity, Exe: "missing"}
	err := startProjectCreation(path, e, func(string, []string, string) error { return errors.New("failed") })
	if err == nil || len(pendingProjects()) != 0 {
		t.Fatalf("failed launch left a pending intent: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("project directory should not be deleted: %v", err)
	}
}

func TestForgetProjectCreationKeepsFiles(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "Game")
	e := editorInstall{Version: "6000.0.2f1", Kind: kindUnity, Exe: "editor"}
	if err := startProjectCreation(path, e, func(string, []string, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	pending := pendingProjects()
	a := newApp()
	a.pendingProjectList = pending
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.NewProjectWaiting() + " · " + path) {
		t.Fatalf("pending project not visible: %q", tt.Texts())
	}
	if err := tt.Click(locales.NewProjectForget()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(pendingProjects()) != 0 || len(a.pendingProjectList) != 0 {
		t.Fatal("intent was not forgotten")
	}
	if _, err := os.Stat(filepath.Join(path, "keep.txt")); err != nil {
		t.Fatal("project data was removed")
	}
}
