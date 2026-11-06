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
