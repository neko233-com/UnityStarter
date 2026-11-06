//go:build windows

package main

import (
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
