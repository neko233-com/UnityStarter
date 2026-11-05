//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// hubConfigDirs are where Unity Hub and Tuanjie Hub keep their files.
func hubConfigDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, base := range []string{
		filepath.Join(home, "Library", "Application Support"),
		filepath.Join(home, ".config"),
	} {
		dirs = append(dirs,
			filepath.Join(base, "UnityHub"),
			filepath.Join(base, "TuanjieHub"),
		)
	}
	return dirs
}

func defaultEditorRoots() []string {
	var roots []string
	for _, hub := range hubConfigDirs() {
		roots = append(roots, readPathJSON(filepath.Join(hub, "secondaryInstallPath.json"))...)
	}
	return roots
}

func defaultProjectRoots() []string {
	var roots []string
	for _, hub := range hubConfigDirs() {
		var configured struct {
			DirectoryPath string `json:"directoryPath"`
		}
		if data, err := os.ReadFile(filepath.Join(hub, "projectDir.json")); err == nil {
			if json.Unmarshal(data, &configured) == nil {
				roots = append(roots, configured.DirectoryPath)
			}
		}
	}
	return roots
}

// readPathJSON reads a path hub configuration file: a single JSON string, or
// a list of them.
func readPathJSON(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var one string
	if json.Unmarshal(data, &one) == nil && one != "" {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(data, &many) == nil {
		return many
	}
	return nil
}
