//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// defaultEditorRoots are where Unity Hub and Tuanjie Hub install editors,
// next to the roots both hubs keep in their configuration.
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

// hubProjects reads the projects Unity Hub knows, which also says when each
// was last opened.
func hubProjects() map[string]projectHint {
	out := map[string]projectHint{}
	for _, hub := range hubConfigDirs() {
		var file struct {
			Data map[string]struct {
				Title        string `json:"title"`
				LastModified int64  `json:"lastModified"`
				Version      string `json:"version"`
			} `json:"data"`
		}
		data, err := os.ReadFile(filepath.Join(hub, "projects-v1.json"))
		if err != nil || json.Unmarshal(data, &file) != nil {
			continue
		}
		for path, p := range file.Data {
			hint := projectHint{Title: p.Title, Version: p.Version}
			if p.LastModified > 0 {
				hint.Modified = time.UnixMilli(p.LastModified)
			}
			out[path] = hint
		}
	}
	return out
}

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
