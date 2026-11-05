//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// Editors of other platforms live in application folders the user adds, as
// the hubs of macOS and Linux keep their own lists.
func defaultEditorRoots() []string { return nil }

func defaultProjectRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, "UnityProjects")}
}

func hubProjects() map[string]projectHint { return nil }
