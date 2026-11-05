package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editorsUnder keeps the editors found under a folder, so that the editors of
// this machine do not mix with a test's.
func editorsUnder(root string) []editorInstall {
	var mine []editorInstall
	for _, e := range scanEditors([]string{root}) {
		if strings.HasPrefix(e.Root, root) {
			mine = append(mine, e)
		}
	}
	return mine
}

// A stand-in editor has the layout a real one has, so scanning finds it, and
// it is marked as simulated.
func TestSimulateEditorInstall(t *testing.T) {
	root := t.TempDir()
	dir, err := simulateEditorInstall(kindUnity, "6000.0.85f2", root)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "6000.0.85f2") {
		t.Errorf("dir %s", dir)
	}
	editors := editorsUnder(root)
	if len(editors) != 1 {
		t.Fatalf("editors %v", editors)
	}
	e := editors[0]
	if e.Version != "6000.0.85f2" || e.Kind != kindUnity || !e.Simulated {
		t.Errorf("editor %+v", e)
	}
	if _, err := os.Stat(e.Exe); err != nil {
		t.Errorf("the executable is missing: %v", err)
	}

	if err := removeSimulatedEditor(dir); err != nil {
		t.Fatal(err)
	}
	if len(editorsUnder(root)) != 0 {
		t.Error("the stand-in is still found")
	}
	if err := removeSimulatedEditor(t.TempDir()); err == nil {
		t.Error("a real folder was removed")
	}
}

func TestSimulateTuanjie(t *testing.T) {
	root := t.TempDir()
	if _, err := simulateEditorInstall(kindTuanjie, "2022.3.99t1", root); err != nil {
		t.Fatal(err)
	}
	editors := editorsUnder(root)
	if len(editors) != 1 || editors[0].Kind != kindTuanjie || filepath.Base(editors[0].Exe) != "Tuanjie.exe" {
		t.Errorf("editors %v", editors)
	}
}

func TestSimulateDoesNotOverwriteExistingEditor(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "7000.0.0a16", "Editor", "Unity.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("real editor"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := simulateEditorInstall(kindUnity, "7000.0.0a16", root); err == nil {
		t.Fatal("simulation replaced an existing editor")
	}
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "real editor" {
		t.Fatalf("editor changed: %q, %v", data, err)
	}
}
