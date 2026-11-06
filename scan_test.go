package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
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

func TestScanEditorsKeepsDistinctInstallationsOfSameVersion(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, root := range []string{first, second} {
		dir := filepath.Join(root, "6000.0.2f1", "Editor")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Unity.exe"), []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	editors := scanEditorsIn([]string{first, second, first})
	if len(editors) != 2 {
		t.Fatalf("expected both installations once: %+v", editors)
	}
	if editors[0].Root != filepath.Join(first, "6000.0.2f1") || editors[1].Root != filepath.Join(second, "6000.0.2f1") {
		t.Fatalf("root precedence changed: %+v", editors)
	}
	project := filepath.Join(t.TempDir(), "game")
	writeProject(t, project, "6000.0.2f1")
	projects := scanProjectsInMode(prefs{PinnedProjects: []string{project}}, editors, nil, nil, false)
	if len(projects) != 1 || projects[0].Editor != editors[0].Exe {
		t.Fatalf("preferred editor did not follow root precedence: %+v", projects)
	}
	a := newApp()
	a.editors = editors
	a.projects = projects
	var launched string
	a.launchEditor = func(exe string, _ []string, _ string) error { launched = exe; return nil }
	a.chooseProjectVersion(projects[0])
	if choices := a.projectEditors(projects[0]); len(choices) != 2 || a.versionEditor != editors[0].Exe {
		t.Fatalf("project version picker missing installations: %+v", choices)
	}
	tt := ui.NewTester(a.view, 1180, 760)
	path, ok := tt.Find(editors[1].Exe)
	if !ok {
		t.Fatalf("second editor path missing from picker: %q", tt.Texts())
	}
	tt.ClickAt(path.X-100, path.Y+path.H/2)
	if a.versionEditor != editors[1].Exe {
		t.Fatalf("second editor was not selected: %q", a.versionEditor)
	}
	if err := tt.Click(locales.ProjectVersionOpen()); err != nil {
		t.Fatal(err)
	}
	if launched != editors[1].Exe {
		t.Fatalf("wrong editor launched: %q", launched)
	}
}

func TestScanEditorFromVersionOrEditorFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "6000.0.2f1")
	editorDir := filepath.Join(root, "Editor")
	if err := os.MkdirAll(editorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(editorDir, "Unity.exe"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{root, editorDir, filepath.Dir(root)} {
		editors := scanEditorsIn([]string{selected})
		if len(editors) != 1 || editors[0].Root != root || editors[0].Version != "6000.0.2f1" {
			t.Fatalf("selecting %q: %+v", selected, editors)
		}
	}
}

func TestEditorProductVersion(t *testing.T) {
	for product, want := range map[string]string{
		"7000.0.0a16_0745a91df592":  "7000.0.0a16",
		"2022.3.62t17_f267bc08c82e": "2022.3.62t17",
		"6000.0.1f1":                "6000.0.1f1",
		"7000.0.0.476585":           "",
		"7000.0.0a16_untrusted":     "",
	} {
		if got := parseEditorProductVersion(product); got != want {
			t.Errorf("%q: got %q, want %q", product, got, want)
		}
	}
}

func TestLiveEditorVersionInCustomFolder(t *testing.T) {
	source := os.Getenv("UNITYSTARTER_LIVE_EDITOR")
	if source == "" {
		t.Skip("set UNITYSTARTER_LIVE_EDITOR to a real Unity executable")
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "custom installation")
	exe := filepath.Join(root, "Editor", "Unity.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, content, 0o755); err != nil {
		t.Fatal(err)
	}
	editors := scanEditorsIn([]string{root})
	if len(editors) != 1 || editors[0].Version != "7000.0.0a16" || editors[0].Exe != exe {
		t.Fatalf("real editor version was not read from executable: %+v", editors)
	}
}

func TestLiveTuanjieEditorVersion(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_LIVE_TUANJIE_EDITOR")
	if exe == "" {
		t.Skip("set UNITYSTARTER_LIVE_TUANJIE_EDITOR to a real Tuanjie executable")
	}
	if got := editorExecutableVersion(exe); got != "2022.3.62t17" {
		t.Fatalf("real Tuanjie product version: %q", got)
	}
	editors := scanEditorsIn([]string{filepath.Dir(exe)})
	if len(editors) != 1 || editors[0].Kind != kindTuanjie || editors[0].Version != "2022.3.62t17" {
		t.Fatalf("real Tuanjie editor scan: %+v", editors)
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
	for _, pair := range [][2]string{
		{"7000.0.0f1", "7000.0.0b99"},
		{"7000.0.0b1", "7000.0.0a16"},
		{"2022.3.62t17", "2022.3.62t2"},
		{"6000.0.0p1", "6000.0.0f99"},
	} {
		if compareVersions(pair[0], pair[1]) <= 0 {
			t.Errorf("%s should sort after %s", pair[0], pair[1])
		}
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

func TestProjectVersionDeterminesEngineEvenWhenListedByOtherHub(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "tuanjie-project")
	writeProject(t, project, "2022.3.62t17")
	h := hub{Kind: kindUnity, Registry: filepath.Join(dir, "projects-v1.json"), Favorites: filepath.Join(dir, "favoriteProjects.json")}
	if err := h.addProject(projectEntry{Path: project, Name: "tuanjie-project", Version: "2022.3.62t17"}); err != nil {
		t.Fatal(err)
	}
	projects := scanProjectsIn(prefs{}, nil, []hub{h}, nil)
	if len(projects) != 1 || projects[0].Kind != kindTuanjie {
		t.Fatalf("project engine: %+v", projects)
	}
}
