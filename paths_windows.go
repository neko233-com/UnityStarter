//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// hubConfigDirs are where Unity Hub and Tuanjie Hub keep their files.
func hubConfigDirs() []string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}
	return []string{
		filepath.Join(appData, "UnityHub"),
		filepath.Join(appData, "TuanjieHub"),
	}
}

// defaultEditorRoots are where both hubs install editors.
func defaultEditorRoots() []string {
	var roots []string
	for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if dir == "" {
			continue
		}
		roots = append(roots,
			filepath.Join(dir, "Unity", "Hub", "Editor"),
			filepath.Join(dir, "Tuanjie", "Hub", "Editor"),
		)
	}
	for _, hub := range hubConfigDirs() {
		roots = append(roots, readPathJSON(filepath.Join(hub, "secondaryInstallPath.json"))...)
	}
	return roots
}

// Versions installed by older releases remain discoverable.
func legacyInstallRoots() []string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return []string{filepath.Join(local, "Programs", "UnityStarter", "Editors")}
	}
	return nil
}

// defaultProjectRoots are the folders each hub lists projects from.
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

// defaultInstallRoot is a folder this user may write to, where downloaded
// editors are installed.
func defaultInstallRoot() string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "UnityStarter", "Editors")
	}
	return filepath.Join(os.TempDir(), "UnityStarter", "Editors")
}
