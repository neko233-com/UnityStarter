package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEditorRecentProjectNamesAndFiltering(t *testing.T) {
	if got := editorPrefValueName(0); got != "RecentlyUsedProjectPaths-0_h1085040554" {
		t.Fatal(got)
	}
	if got := editorPrefValueName(10); got != "RecentlyUsedProjectPaths-10_h1446599995" {
		t.Fatal(got)
	}
	values := map[string][]byte{
		editorPrefValueName(0): []byte("D:/Game/Project\x00"),
		editorPrefValueName(1): []byte("d:\\game\\project\x00"),
		editorPrefValueName(2): []byte("bad\x00path"),
		editorPrefValueName(3): {0xff},
	}
	got := collectRecentEditorProjects(kindTuanjie, func(name string) []byte { return values[name] })
	if len(got) != 1 || got[0].Path != "D:/Game/Project" || got[0].Kind != kindTuanjie {
		t.Fatalf("unexpected recent projects: %+v", got)
	}
}

func TestLiveRecentEditorProject(t *testing.T) {
	want := os.Getenv("UNITYSTARTER_EXPECT_RECENT_PROJECT")
	if want == "" {
		t.Skip("set UNITYSTARTER_EXPECT_RECENT_PROJECT to check a real editor registry entry")
	}
	want = filepath.Clean(want)
	start := time.Now()
	_, projects, _ := scanWithVCS(prefs{}, false)
	t.Logf("project discovery without blocking VCS: %d projects in %s", len(projects), time.Since(start).Round(time.Millisecond))
	for _, project := range projects {
		if strings.EqualFold(filepath.Clean(project.Path), want) {
			if project.Kind != kindTuanjie {
				t.Fatalf("wrong engine: %+v", project)
			}
			return
		}
	}
	t.Fatalf("recent project %s not found", want)
}

func TestRecentTemporaryProjectsAndExplicitAdditions(t *testing.T) {
	base := t.TempDir()
	tempRoot := filepath.Join(base, "Temp")
	temporary := filepath.Join(tempRoot, "UniCli", "Project")
	regular := filepath.Join(base, "TempProject", "Game")
	writeProject(t, temporary, "2022.3.62t17")
	writeProject(t, regular, "2022.3.62t17")
	recent := []recentEditorProject{{Path: temporary, Kind: kindTuanjie}, {Path: regular, Kind: kindTuanjie}}
	projects := scanProjectsWithRecent(prefs{}, nil, nil, nil, recent, tempRoot, false)
	if len(projects) != 1 || projects[0].Path != regular {
		t.Fatalf("automatic temporary project was not filtered: %+v", projects)
	}
	projects = scanProjectsWithRecent(prefs{PinnedProjects: []string{temporary}}, nil, nil, nil, recent, tempRoot, false)
	if len(projects) != 2 {
		t.Fatalf("explicitly added temporary project was lost: %+v", projects)
	}
	projects = scanProjectsWithRecent(prefs{LocalFavorites: map[string]bool{localProjectKey(temporary): true}}, nil, nil, nil, recent, tempRoot, false)
	if len(projects) != 2 {
		t.Fatalf("explicitly favorited temporary project was lost: %+v", projects)
	}
	projects = scanProjectsWithRecent(prefs{}, nil, nil, []string{temporary}, recent, tempRoot, false)
	if len(projects) != 2 {
		t.Fatalf("explicit project root was lost: %+v", projects)
	}
	h := hub{Kind: kindUnity, Registry: filepath.Join(base, "projects-v1.json"), Favorites: filepath.Join(base, "favoriteProjects.json")}
	if err := h.addProject(projectEntry{Path: temporary, Name: "intentional", Version: "2022.3.62t17"}); err != nil {
		t.Fatal(err)
	}
	projects = scanProjectsWithRecent(prefs{}, nil, []hub{h}, nil, recent, tempRoot, false)
	if len(projects) != 2 {
		t.Fatalf("Hub project was lost: %+v", projects)
	}
}

func TestRecentCodexWorktreesAndExplicitAdditions(t *testing.T) {
	base := t.TempDir()
	worktrees := filepath.Join(base, ".codex", "worktrees")
	worktree := filepath.Join(worktrees, "task", "Game")
	regular := filepath.Join(base, "Game")
	independent := filepath.Join(base, "other", "Game")
	sibling := filepath.Join(base, ".codex", "worktrees-backup", "Game")
	for _, path := range []string{worktree, regular, independent, sibling} {
		writeProject(t, path, "2022.3.62t17")
	}
	recent := []recentEditorProject{{Path: worktree}, {Path: regular}, {Path: independent}, {Path: sibling}}
	scan := func(p prefs, hubs []hub, roots []string) []projectEntry {
		return scanProjectsWithRecentRoots(p, nil, hubs, roots, recent, filepath.Join(base, "temp"), []string{worktrees}, false)
	}
	if got := scan(prefs{}, nil, nil); len(got) != 3 {
		t.Fatalf("automatic Codex worktree should be excluded; independent same-named projects should remain: %+v", got)
	}
	if got := scan(prefs{PinnedProjects: []string{worktree}}, nil, nil); len(got) != 4 {
		t.Fatalf("explicitly added worktree missing: %+v", got)
	}
	if got := scan(prefs{LocalFavorites: map[string]bool{localProjectKey(worktree): true}}, nil, nil); len(got) != 4 {
		t.Fatalf("favorited worktree missing: %+v", got)
	}
	if got := scan(prefs{}, nil, []string{worktree}); len(got) != 4 {
		t.Fatalf("explicit worktree root missing: %+v", got)
	}
	h := hub{Kind: kindUnity, Registry: filepath.Join(base, "projects-v1.json"), Favorites: filepath.Join(base, "favoriteProjects.json")}
	if err := h.addProject(projectEntry{Path: worktree, Name: "intentional", Version: "2022.3.62t17"}); err != nil {
		t.Fatal(err)
	}
	if got := scan(prefs{}, []hub{h}, nil); len(got) != 4 {
		t.Fatalf("Hub worktree missing: %+v", got)
	}
}

func TestGeneratedProjectsInEditorRecentHistory(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "Game")
	generated := filepath.Join(parent, "Library", "UniCli", "workbench", "Project~")
	temporary := filepath.Join(parent, "Temp", "Project")
	legitimate := filepath.Join(base, "Library", "Game")
	for _, project := range []string{parent, generated, temporary, legitimate} {
		writeProject(t, project, "2022.3.62t17")
	}
	recent := []recentEditorProject{{Path: parent}, {Path: generated}, {Path: temporary}, {Path: legitimate}}
	scan := func(p prefs, hubs []hub, roots []string) []projectEntry {
		return scanProjectsWithRecent(p, nil, hubs, roots, recent, filepath.Join(base, "unrelated-temp"), false)
	}
	if got := scan(prefs{}, nil, nil); len(got) != 2 {
		t.Fatalf("generated recent projects must be excluded; standalone Library project must remain: %+v", got)
	}
	if got := scan(prefs{PinnedProjects: []string{generated}}, nil, nil); len(got) != 3 {
		t.Fatalf("explicitly added project missing: %+v", got)
	}
	if got := scan(prefs{LocalFavorites: map[string]bool{localProjectKey(generated): true}}, nil, nil); len(got) != 3 {
		t.Fatalf("favorited project missing: %+v", got)
	}
	if got := scan(prefs{}, nil, []string{generated}); len(got) != 3 {
		t.Fatalf("explicit project root missing: %+v", got)
	}
	h := hub{Kind: kindUnity, Registry: filepath.Join(base, "projects-v1.json"), Favorites: filepath.Join(base, "favoriteProjects.json")}
	if err := h.addProject(projectEntry{Path: generated, Name: "intentional", Version: "2022.3.62t17"}); err != nil {
		t.Fatal(err)
	}
	if got := scan(prefs{}, []hub{h}, nil); len(got) != 3 {
		t.Fatalf("Hub project missing: %+v", got)
	}
}

func TestLiveRecentTemporaryFilter(t *testing.T) {
	want := os.Getenv("UNITYSTARTER_EXPECT_TEMP_PROJECT")
	if want == "" {
		t.Skip("set UNITYSTARTER_EXPECT_TEMP_PROJECT to check a real editor recent entry")
	}
	recent := recentEditorProjects()
	found := false
	for _, entry := range recent {
		if strings.EqualFold(filepath.Clean(entry.Path), filepath.Clean(want)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("target is not in editor recent history")
	}
	if !pathWithinDirectory(want, os.TempDir()) {
		t.Fatalf("temporary directory not recognized: %s", os.TempDir())
	}
	projects := scanProjectsWithRecent(prefs{}, nil, nil, nil, recent, os.TempDir(), false)
	for _, p := range projects {
		if strings.EqualFold(filepath.Clean(p.Path), filepath.Clean(want)) {
			t.Fatal("temporary recent project remains visible")
		}
	}
	_, actual, _ := scanWithVCS(loadPrefs(), false)
	for _, p := range actual {
		if strings.EqualFold(filepath.Clean(p.Path), filepath.Clean(want)) {
			t.Fatal("temporary project remains in the real project list")
		}
	}
	t.Logf("real project scan contains %d projects and excludes the temporary entry", len(actual))
	projects = scanProjectsWithRecent(prefs{PinnedProjects: []string{want}}, nil, nil, nil, recent, os.TempDir(), false)
	for _, p := range projects {
		if strings.EqualFold(filepath.Clean(p.Path), filepath.Clean(want)) {
			t.Logf("confirmed editor recent source, filtered automatically and retained when explicitly added")
			return
		}
	}
	t.Fatal("explicitly added temporary project missing")
}
