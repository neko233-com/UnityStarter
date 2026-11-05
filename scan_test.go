package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeProject(t *testing.T, dir, version string) {
	t.Helper()
	settings := filepath.Join(dir, "ProjectSettings")
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "m_EditorVersion: " + version + "\nm_EditorVersionWithRevision: " + version + " (9f9d16c45e54)\n"
	if err := os.WriteFile(filepath.Join(settings, "ProjectVersion.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadProject(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, dir, "2022.3.51f1")
	version, info, ok := readProject(dir)
	if !ok || version != "2022.3.51f1" {
		t.Fatalf("version %q, ok %v", version, ok)
	}
	if info == nil || time.Since(info.ModTime()) > time.Minute {
		t.Errorf("info %v", info)
	}
	if _, _, ok := readProject(t.TempDir()); ok {
		t.Error("a folder without ProjectSettings read as a project")
	}
}

func TestWalkProjects(t *testing.T) {
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "one"), "2022.3.51f1")
	writeProject(t, filepath.Join(root, "group", "two"), "2022.3.62t17")
	// Folders the walk must not go into are not projects.
	if err := os.MkdirAll(filepath.Join(root, "one", "Library", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeProject(t, filepath.Join(root, "one", "Library", "deep", "nested"), "2022.3.51f1")
	// Too deep to be found.
	writeProject(t, filepath.Join(root, "a", "b", "c", "d", "e", "deep"), "2022.3.51f1")

	var found []string
	walkProjects(root, 3, func(dir string, _ projectHint) {
		found = append(found, filepath.Base(dir))
	})
	if len(found) != 2 {
		t.Fatalf("found %v", found)
	}
}

func TestKindOfVersion(t *testing.T) {
	cases := map[string]string{
		"2022.3.51f1":  kindUnity,
		"6000.0.23f1":  kindUnity,
		"2022.3.62t17": kindTuanjie,
		"2022.3.2t3":   kindTuanjie,
	}
	for version, want := range cases {
		if got := kindOfVersion(version); got != want {
			t.Errorf("%s: %s, want %s", version, got, want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("2022.3.51f1", "2022.3.9f1") <= 0 {
		t.Error("51 sorts before 9")
	}
	if compareVersions("2022.3.51f1", "2022.3.51f1") != 0 {
		t.Error("same versions differ")
	}
	if compareVersions("2021.3.1f1", "2022.3.51f1") >= 0 {
		t.Error("2021 sorts after 2022")
	}
}

// Projects a hub lists are imported, and its favorites are kept.
func TestScanProjectsFromHub(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, filepath.Join(dir, "fromhub"), "2022.3.51f1")
	h := hub{
		Kind:      kindUnity,
		Dir:       dir,
		Registry:  filepath.Join(dir, "projects-v1.json"),
		Favorites: filepath.Join(dir, "favoriteProjects.json"),
	}
	if err := h.addProject(projectEntry{Path: filepath.Join(dir, "fromhub"), Name: "renamed", Version: "2022.3.51f1"}); err != nil {
		t.Fatal(err)
	}
	if err := h.setFavorite(filepath.Join(dir, "fromhub"), true); err != nil {
		t.Fatal(err)
	}
	projects := scanProjectsIn(prefs{}, nil, []hub{h}, nil)
	if len(projects) != 1 {
		t.Fatalf("projects %v", projects)
	}
	p := projects[0]
	if p.Name != "renamed" || p.Hub != kindUnity || !p.Favorite {
		t.Errorf("project %+v", p)
	}
}
