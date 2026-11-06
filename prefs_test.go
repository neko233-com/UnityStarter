package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := prefs{
		Locale:         "zh-Hans",
		ProjectRoots:   []string{`D:\Games`},
		EditorRoots:    []string{`D:\IDE\UnityEditor`},
		PinnedProjects: []string{`D:\Games\demo`},
		NoHubSync:      true,
	}
	if err := savePrefsTo(path, want); err != nil {
		t.Fatal(err)
	}
	got := loadPrefsFrom(path)
	if got.Locale != want.Locale || !got.NoHubSync ||
		len(got.ProjectRoots) != 1 || got.ProjectRoots[0] != want.ProjectRoots[0] ||
		len(got.PinnedProjects) != 1 {
		t.Errorf("prefs %+v", got)
	}
	// A broken file reads as no settings rather than as an error.
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadPrefsFrom(path); got.Locale != "" {
		t.Errorf("broken settings read as %+v", got)
	}
}

func TestLastEngineIsRestored(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := savePrefs(prefs{ActiveEngine: kindTuanjie}); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.load()
	if a.activeEngine != kindTuanjie || a.page != pageTuanjieProjects {
		t.Fatalf("restored engine=%s page=%s", a.activeEngine, a.page)
	}
	a.switchEngine(kindUnity)
	if a.prefs.ActiveEngine != kindUnity || a.page != pageProjects {
		t.Fatalf("switched engine=%s page=%s", a.prefs.ActiveEngine, a.page)
	}
}

// The hubs add the projects added by hand, and leave the others alone.
func TestHubSyncCommand(t *testing.T) {
	dir := t.TempDir()
	h := hub{
		Kind:      kindUnity,
		Dir:       dir,
		Registry:  filepath.Join(dir, "projects-v1.json"),
		Favorites: filepath.Join(dir, "favoriteProjects.json"),
	}
	if err := os.WriteFile(h.Favorites, []byte(`"[]"`), 0o644); err != nil {
		t.Fatal(err)
	}
	pinned := projectEntry{Path: `D:\Games\pinned`, Name: "pinned", Version: "2022.3.51f1", Kind: kindUnity}
	other := projectEntry{Path: `D:\Games\other`, Name: "other", Version: "2022.3.51f1", Kind: kindUnity}
	scanFn := func(prefs) scanResult {
		return scanResult{projects: []projectEntry{pinned, other}, hubs: []hub{h}}
	}

	var buf bytes.Buffer
	settings := prefs{PinnedProjects: []string{pinned.Path}}
	if code := hubSync(settings, cliOptions{json: true}, scanFn, &buf); code != 0 {
		t.Fatalf("exit %d: %s", code, buf.String())
	}
	var got struct{ Added, Pinned int }
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if got.Added != 1 || got.Pinned != 1 {
		t.Errorf("result %+v", got)
	}
	if !h.lists(pinned.Path) {
		t.Error("the pinned project is not in the hub")
	}
	if h.lists(other.Path) {
		t.Error("a project not added by hand went to the hub")
	}

	// Syncing again adds nothing.
	buf.Reset()
	if code := hubSync(settings, cliOptions{json: true}, scanFn, &buf); code != 0 {
		t.Fatalf("exit %d: %s", code, buf.String())
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil || got.Added != 0 {
		t.Errorf("second sync %+v (%v)", got, err)
	}

	// Sync off refuses to write.
	settings.NoHubSync = true
	buf.Reset()
	if code := hubSync(settings, cliOptions{}, scanFn, &buf); code != 1 {
		t.Errorf("exit %d with sync off", code)
	}
}
