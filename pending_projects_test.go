package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestProjectRegistrationFailureRemainsVisibleAndRetries(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "Game")
	e := editorInstall{Version: "6000.0.2f1", Kind: kindUnity, Exe: "editor"}
	if err := startProjectCreation(path, e, func(string, []string, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	writeProject(t, path, e.Version)
	for _, dir := range []string{"Assets", "Packages"} {
		if err := os.MkdirAll(filepath.Join(path, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(path, "Packages", "manifest.json"), []byte(`{"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pending := pendingProjects()
	a := newApp()
	a.pendingProjectList = pending
	a.hubs = []hub{{Kind: kindUnity, Registry: t.TempDir()}}
	a.finishPendingProjects(pending)
	if len(pendingProjects()) != 1 || a.pendingProjectErrors[pending[0].file] == "" {
		t.Fatal("failed Hub registration lost the pending request or its error")
	}
	tt := ui.NewTester(a.view, 1180, 760)
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), locales.NewProjectRegisterFailed()) {
		t.Fatalf("registration failure not visible: %q", tt.Texts())
	}
	h := hub{Kind: kindUnity, Registry: filepath.Join(t.TempDir(), "projects-v1.json")}
	a.hubs = []hub{h}
	a.finishPendingProjects(pending)
	if !h.lists(path) || len(pendingProjects()) != 0 || a.pendingProjectErrors[pending[0].file] != "" {
		t.Fatal("retry did not register the project and clear the error")
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

func TestPendingProjectDelayedStatusAndEditorLog(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	now := time.Now()
	p := pendingProject{Path: `C:\Projects\Game`, Kind: kindUnity, CreatedAt: now.Add(-3 * time.Minute)}
	if !pendingProjectDelayed(p, now) || pendingProjectDelayed(p, p.CreatedAt.Add(time.Minute)) {
		t.Fatal("pending delay threshold is incorrect")
	}
	a := newApp()
	a.pendingProjectList = []pendingProject{p}
	tt := ui.NewTester(a.view, 1180, 760)
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), locales.NewProjectWaitingLong()) || !tt.HasText(locales.NewProjectEditorLog()) {
		t.Fatalf("delayed state or log action missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.NewProjectEditorLog()); err != nil {
		t.Fatal(err)
	}
	if !a.alertOpen || a.alertMessage != locales.NewProjectLogMissing() {
		t.Fatal("missing editor log was not reported")
	}
	if got := editorLogPath(kindTuanjie); got != filepath.Join(os.Getenv("LOCALAPPDATA"), "Tuanjie", "Editor", "Editor.log") {
		t.Fatalf("wrong Tuanjie log path: %s", got)
	}
}
