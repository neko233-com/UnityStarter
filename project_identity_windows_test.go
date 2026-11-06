//go:build windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsProjectCaseAliasesMergeAndFavoriteSync(t *testing.T) {
	project := filepath.Join(t.TempDir(), "MyGame")
	other := filepath.Join(t.TempDir(), "MyGame")
	writeProject(t, project, "2022.3.51f1")
	writeProject(t, other, "2022.3.51f1")
	alias := strings.ToUpper(project)
	h := hub{Kind: kindUnity, Registry: filepath.Join(t.TempDir(), "projects-v1.json"), Favorites: filepath.Join(t.TempDir(), "favorites.json")}
	if err := h.addProject(projectEntry{Path: project, Name: "MyGame", Version: "2022.3.51f1"}); err != nil {
		t.Fatal(err)
	}
	if err := h.addProject(projectEntry{Path: alias, Name: "duplicate", Version: "2022.3.51f1"}); err != nil {
		t.Fatal(err)
	}
	_, registry := readRegistry(h.Registry)
	if len(registry) != 1 || !h.lists(alias) {
		t.Fatalf("case alias duplicated Hub entry: %d", len(registry))
	}
	if err := h.setFavorite(alias, true); err != nil {
		t.Fatal(err)
	}
	recent := []recentEditorProject{{Path: project}, {Path: alias}, {Path: other}}
	settings := prefs{PinnedProjects: []string{project, alias, other}}
	projects := scanProjectsWithRecent(settings, nil, []hub{h}, nil, recent, t.TempDir(), false)
	if len(projects) != 2 {
		t.Fatalf("case aliases or distinct projects merged incorrectly: %+v", projects)
	}
	var favorite bool
	for _, p := range projects {
		if localProjectKey(p.Path) == localProjectKey(project) {
			favorite = p.Favorite && p.Hub == kindUnity
		}
	}
	if !favorite {
		t.Fatalf("Hub favorite lost during merge: %+v", projects)
	}
	settings.LocalFavorites = map[string]bool{localProjectKey(project): false}
	for _, p := range scanProjectsWithRecent(settings, nil, []hub{h}, nil, recent, t.TempDir(), false) {
		if localProjectKey(p.Path) == localProjectKey(project) && p.Favorite {
			t.Fatal("explicit local favorite override was lost")
		}
	}
	if err := h.setFavorite(project, false); err != nil {
		t.Fatal(err)
	}
	if len(h.favorites()) != 0 {
		t.Fatal("case alias remained in Hub favorites")
	}
	for _, p := range h.projects() {
		if p.Favorite {
			t.Fatal("registry favorite remained set")
		}
	}
	settings.HiddenProjects = map[string]string{localProjectKey(project): project}
	if projects := scanProjectsWithRecent(settings, nil, []hub{h}, nil, recent, t.TempDir(), false); len(projects) != 1 || localProjectKey(projects[0].Path) != localProjectKey(other) {
		t.Fatalf("hidden project reappeared through a case alias: %+v", projects)
	}
}

func TestWindowsProjectJunctionAliases(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	base := t.TempDir()
	project := filepath.Join(base, "real", "Game")
	other := filepath.Join(base, "other", "Game")
	alias := filepath.Join(base, "alias")
	writeProject(t, project, "2022.3.51f1")
	writeProject(t, other, "2022.3.51f1")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", alias, project).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, out)
	}
	if projectIdentityKey(project) != projectIdentityKey(alias) {
		t.Fatalf("junction did not resolve to project identity: %q != %q", projectIdentityKey(project), projectIdentityKey(alias))
	}
	h := hub{Kind: kindUnity, Registry: filepath.Join(base, "projects-v1.json"), Favorites: filepath.Join(base, "favorites.json")}
	if err := h.addProject(projectEntry{Path: project, Name: "Game", Version: "2022.3.51f1"}); err != nil {
		t.Fatal(err)
	}
	if err := h.addProject(projectEntry{Path: alias, Name: "Game", Version: "2022.3.51f1"}); err != nil {
		t.Fatal(err)
	}
	_, registry := readRegistry(h.Registry)
	if len(registry) != 1 || !h.lists(alias) {
		t.Fatalf("junction duplicated Hub entry: %d", len(registry))
	}
	if err := h.setFavorite(alias, true); err != nil {
		t.Fatal(err)
	}
	settings := prefs{PinnedProjects: []string{alias, project, other}}
	projects := scanProjectsInMode(settings, nil, []hub{h}, nil, false)
	if len(projects) != 2 {
		t.Fatalf("same directory should merge and independent project should remain: %+v", projects)
	}
	for _, p := range projects {
		if projectIdentityKey(p.Path) == projectIdentityKey(project) && !p.Favorite {
			t.Fatal("Hub favorite lost through junction")
		}
	}
	settings.LocalFavorites = map[string]bool{localProjectKey(alias): false}
	for _, p := range scanProjectsInMode(settings, nil, []hub{h}, nil, false) {
		if projectIdentityKey(p.Path) == projectIdentityKey(project) && p.Favorite {
			t.Fatal("local favorite override lost through junction")
		}
	}
	settings.HiddenProjects = map[string]string{localProjectKey(alias): alias}
	projects = scanProjectsInMode(settings, nil, []hub{h}, nil, false)
	if len(projects) != 1 || projectIdentityKey(projects[0].Path) != projectIdentityKey(other) {
		t.Fatalf("hidden project reappeared through junction: %+v", projects)
	}
	if err := h.setFavorite(project, false); err != nil {
		t.Fatal(err)
	}
	if len(h.favorites()) != 0 {
		t.Fatal("junction favorite remained after unstar")
	}
	a := newApp()
	a.prefs.NoHubSync = true
	a.prefs.LocalFavorites = map[string]bool{localProjectKey(alias): true}
	a.toggleFavorite(projectEntry{Path: project, Favorite: true})
	if len(a.prefs.LocalFavorites) != 1 || a.prefs.LocalFavorites[localProjectKey(project)] {
		t.Fatalf("unstar left an old alias override: %+v", a.prefs.LocalFavorites)
	}
	a.prefs.HiddenProjects = map[string]string{localProjectKey(alias): alias}
	a.setProjectHidden(project, false)
	if len(a.prefs.HiddenProjects) != 0 {
		t.Fatalf("restore left an old alias hidden: %+v", a.prefs.HiddenProjects)
	}
}

func TestWindowsJunctionRespectsAutomaticProjectFilters(t *testing.T) {
	base := t.TempDir()
	temporary := filepath.Join(base, "temp", "Game")
	alias := filepath.Join(base, "alias")
	writeProject(t, temporary, "2022.3.62t17")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", alias, temporary).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, out)
	}
	projects := scanProjectsWithRecentRoots(prefs{}, nil, nil, nil, []recentEditorProject{{Path: alias}}, filepath.Join(base, "temp"), nil, false)
	if len(projects) != 0 {
		t.Fatalf("temporary project appeared through junction: %+v", projects)
	}
	parent := filepath.Join(base, "parent")
	generated := filepath.Join(parent, "Library", "work", "Project")
	generatedAlias := filepath.Join(base, "generated-alias")
	writeProject(t, parent, "2022.3.62t17")
	writeProject(t, generated, "2022.3.62t17")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", generatedAlias, generated).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, out)
	}
	if !generatedProjectPath(generatedAlias) {
		t.Fatal("generated project appeared through junction")
	}
}
