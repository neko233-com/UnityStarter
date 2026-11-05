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
