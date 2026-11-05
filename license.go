package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// licenseLocation is a file an editor keeps its license in, or the folder the
// Unity licensing client keeps them in.
type licenseLocation struct {
	Path     string
	IsDir    bool
	Files    int
	Size     int64
	Modified time.Time
}

// licenseLocations finds the licenses installed on this machine, for the
// editors of both engines.
func licenseLocations() []licenseLocation {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(os.Getenv("ProgramData"), "Tuanjie", "Tuanjie_lic.ulf"),
			filepath.Join(os.Getenv("ProgramData"), "Unity", "Unity_lic.ulf"),
			filepath.Join(os.Getenv("APPDATA"), "Unity", "Unity_lic.ulf"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Unity", "licenses"),
		}
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			candidates = []string{
				filepath.Join(home, ".local", "share", "unity3d", "Unity", "Unity_lic.ulf"),
				filepath.Join(home, ".config", "unity3d", "Unity", "Unity_lic.ulf"),
				filepath.Join(home, "Library", "Application Support", "Unity", "Unity_lic.ulf"),
			}
		}
	}

	var out []licenseLocation
	for _, path := range candidates {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		loc := licenseLocation{Path: path, IsDir: info.IsDir(), Size: info.Size(), Modified: info.ModTime()}
		if loc.IsDir {
			entries, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			loc.Files = len(entries)
			loc.Size = 0
			for _, entry := range entries {
				if entryInfo, err := entry.Info(); err == nil {
					loc.Size += entryInfo.Size()
				}
			}
			if loc.Files == 0 {
				continue
			}
		}
		out = append(out, loc)
	}
	return out
}
