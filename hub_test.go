package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A hub's files are read and written as the hub writes them, so that both
// see the same projects and favorites.
func TestHubSync(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "projects-v1.json")
	favorites := filepath.Join(dir, "favoriteProjects.json")
	existing := `{"schema_version":"v1","data":{` +
		`"D:\\Games\\a":{"title":"a","path":"D:\\Games\\a","version":"2022.3.51f1",` +
		`"lastModified":1000,"isFavorite":false,"unknown":"kept"}}}`
	if err := os.WriteFile(registry, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(favorites, []byte(`"[]"`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := hub{Kind: kindUnity, Dir: dir, Registry: registry, Favorites: favorites}

	if got := h.projects(); len(got) != 1 {
		t.Fatalf("projects %v", got)
	}

	added := projectEntry{Path: `D:\Games\b`, Name: "b", Version: "2022.3.62t17"}
	if err := h.addProject(added); err != nil {
		t.Fatal(err)
	}
	if got := h.projects(); len(got) != 2 {
		t.Fatalf("projects after adding: %v", got)
	}
	raw, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"unknown":"kept"`) {
		t.Errorf("the hub's own fields were lost: %s", raw)
	}

	if err := h.setFavorite(added.Path, true); err != nil {
		t.Fatal(err)
	}
	if !h.projects()[added.Path].Favorite {
		t.Error("the star was not kept")
	}
	data, err := os.ReadFile(favorites)
	if err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatalf("favorites are not a JSON string, as the hub writes them: %s", data)
	}
	var paths []string
	if err := json.Unmarshal([]byte(encoded), &paths); err != nil {
		t.Fatalf("favorites are not a JSON array: %s", encoded)
	}
	if len(paths) != 1 || paths[0] != added.Path {
		t.Errorf("favorites %v", paths)
	}

	if err := h.setFavorite(added.Path, false); err != nil {
		t.Fatal(err)
	}
	if h.projects()[added.Path].Favorite {
		t.Error("the star was not removed")
	}
}

func TestTuanjieFavoriteUsesTuanjieHub(t *testing.T) {
	unityDir, tuanjieDir := t.TempDir(), t.TempDir()
	unity := hub{Kind: kindUnity, Registry: filepath.Join(unityDir, "projects-v1.json"), Favorites: filepath.Join(unityDir, "favoriteProjects.json")}
	tuanjie := hub{Kind: kindTuanjie, Favorites: filepath.Join(tuanjieDir, "favoriteProjects.json")}
	p := projectEntry{Path: filepath.Join(t.TempDir(), "project"), Kind: kindTuanjie, Hub: kindUnity}
	a := newApp()
	a.hubs = []hub{unity, tuanjie}
	a.projects = []projectEntry{p}
	a.toggleFavorite(p)
	if paths := tuanjie.favorites(); len(paths) != 1 || paths[0] != p.Path {
		t.Fatalf("Tuanjie favorites: %v", paths)
	}
	if paths := unity.favorites(); len(paths) != 0 {
		t.Fatalf("Unity favorites unexpectedly changed: %v", paths)
	}
	if a.projects[0].Hub != kindTuanjie || !a.projects[0].Favorite {
		t.Fatalf("project state: %+v", a.projects[0])
	}
	a.prefs.NoHubSync = true
	if _, ok := a.hubFor(p); ok {
		t.Fatal("disabled hub sync still selected a hub")
	}
}

func TestFavoritesWorkWithoutHubAndWithSyncDisabled(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := t.TempDir()
	writeProject(t, path, "2022.3.62t17")
	p := projectEntry{Path: path, Kind: kindTuanjie}
	a := newApp()
	a.projects = []projectEntry{p}
	a.toggleFavorite(p)
	if !a.projects[0].Favorite {
		t.Fatal("local favorite not enabled")
	}
	stored := loadPrefs()
	stored.PinnedProjects = []string{path}
	projects := scanProjectsIn(stored, nil, nil, nil)
	if len(projects) != 1 || !projects[0].Favorite {
		t.Fatalf("local favorite lost after rescan: %+v", projects)
	}
	a.prefs.NoHubSync = true
	a.toggleFavorite(a.projects[0])
	stored = loadPrefs()
	stored.PinnedProjects = []string{path}
	projects = scanProjectsIn(stored, nil, nil, nil)
	if len(projects) != 1 || projects[0].Favorite {
		t.Fatalf("local favorite did not clear: %+v", projects)
	}
}

// A hub that keeps no project list is read through its favorites, and
// refuses to be written.
func TestHubWithoutRegistry(t *testing.T) {
	dir := t.TempDir()
	favorites := filepath.Join(dir, "favoriteProjects.json")
	if err := os.WriteFile(favorites, []byte(`"[\"D:\\\\Code\\\\c\"]"`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := hub{Kind: kindTuanjie, Dir: dir, Favorites: favorites}

	projects := h.projects()
	if !projects[`D:\Code\c`].Favorite {
		t.Fatalf("projects %v", projects)
	}
	if err := h.addProject(projectEntry{Path: `D:\Code\d`}); err == nil {
		t.Error("a hub without a registry was written")
	}
	if err := h.setFavorite(`D:\Code\c`, true); err != nil {
		t.Errorf("favorites of read-only hub: %v", err)
	}
}
