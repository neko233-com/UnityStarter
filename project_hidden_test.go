package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestHiddenProjectPersistsAndRestoresAcrossSources(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	project := filepath.Join(root, "Game")
	writeProject(t, project, "2022.3.51f1")
	h := hub{Kind: kindUnity, Registry: filepath.Join(root, "projects-v1.json"), Favorites: filepath.Join(root, "favorites.json")}
	if err := h.addProject(projectEntry{Path: project, Name: "Game", Version: "2022.3.51f1"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(h.Registry)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.prefs.PinnedProjects = []string{project}
	a.setProjectHidden(project, true)
	settings := loadPrefs()
	recent := []recentEditorProject{{Path: project, Kind: kindUnity}}
	if got := scanProjectsWithRecent(settings, nil, []hub{h}, []string{root}, recent, filepath.Join(root, "unused"), false); len(got) != 0 {
		t.Fatalf("hidden project reappeared: %+v", got)
	}
	a.setProjectHidden(project, false)
	settings = loadPrefs()
	if got := scanProjectsWithRecent(settings, nil, []hub{h}, []string{root}, recent, filepath.Join(root, "unused"), false); len(got) != 1 || got[0].Path != project {
		t.Fatalf("restored project not found: %+v", got)
	}
	after, err := os.ReadFile(h.Registry)
	if err != nil || string(before) != string(after) || !isProject(project) {
		t.Fatal("hiding or restoring modified project files or Hub registry")
	}
}

func TestProjectMenuHideAndRestore(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := fixture()
	a.page = pageProjects
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("⋯"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.ProjectHide()) {
		t.Fatalf("hide menu missing: %q", tt.Texts())
	}
	if path := os.Getenv("PROJECT_MENU_SNAPSHOT"); path != "" {
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click(locales.ProjectHide()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.filteredProjects()) != 0 || !a.projectHidden(a.projects[0].Path) {
		t.Fatal("project still visible")
	}
	a.page = pageSettings
	tt.Frame()
	if !tt.HasText(locales.HiddenProjectsTitle()) || !tt.HasText(a.projects[0].Path) {
		t.Fatalf("hidden projects settings missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.ProjectRestore()); err != nil {
		t.Fatal(err)
	}
	a.page = pageProjects
	tt.Frame()
	if len(a.filteredProjects()) != 1 {
		t.Fatal("restored project missing")
	}
}
