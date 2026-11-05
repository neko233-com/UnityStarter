//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func TestEditorRootsAreSeparateFromAppInstall(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	if got, want := defaultInstallRoot(), filepath.Join(local, "UnityStarter", "Editors"); got != want {
		t.Fatalf("default install root %q, want %q", got, want)
	}
	legacy := legacyInstallRoots()
	if len(legacy) != 1 || legacy[0] != filepath.Join(local, "Programs", "UnityStarter", "Editors") {
		t.Fatalf("legacy roots %v", legacy)
	}
}
