package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// A creation request survives app/CLI exit without touching shared settings.
type pendingProject struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	file    string
}

func pendingProjectFile(dir string) string {
	sum := sha256.Sum256([]byte(localProjectKey(dir)))
	return filepath.Join(userDataDir(), "pending-projects", hex.EncodeToString(sum[:])+".json")
}

func startProjectCreation(dir string, editor editorInstall, launch func(string, []string, string) error) error {
	if editor.Version == "" || editor.Exe == "" || (editor.Kind != kindUnity && editor.Kind != kindTuanjie) || kindOfVersion(editor.Version) != editor.Kind {
		return fmt.Errorf("invalid editor version or engine")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	intent := pendingProject{Path: dir, Version: editor.Version, Kind: editor.Kind}
	file := pendingProjectFile(dir)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("project creation is already pending or could not be recorded: %w", err)
	}
	err = json.NewEncoder(f).Encode(intent)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(file)
		return err
	}
	if err = launch(editor.Exe, []string{"-createProject", dir}, filepath.Dir(editor.Exe)); err != nil {
		_ = os.Remove(file)
		return err
	}
	return nil
}

func pendingProjects() []pendingProject {
	dir := filepath.Join(userDataDir(), "pending-projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var pending []pendingProject
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		file := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(file)
		if err != nil || len(data) > 16384 {
			continue
		}
		var p pendingProject
		if json.Unmarshal(data, &p) != nil || !filepath.IsAbs(p.Path) || p.Version == "" || (p.Kind != kindUnity && p.Kind != kindTuanjie) || !strings.EqualFold(file, pendingProjectFile(p.Path)) {
			continue
		}
		p.file = file
		pending = append(pending, p)
	}
	return pending
}

// A version file alone can appear while the editor is still creating files.
// These checks prove a usable project structure exists, not that import or
// licensing completed successfully.
func pendingProjectReady(p pendingProject) bool {
	version, _, ok := readProject(p.Path)
	if !ok || !strings.EqualFold(version, p.Version) || kindOfVersion(version) != p.Kind {
		return false
	}
	assets, err := os.Stat(filepath.Join(p.Path, "Assets"))
	if err != nil || !assets.IsDir() {
		return false
	}
	manifestPath := filepath.Join(p.Path, "Packages", "manifest.json")
	manifest, err := os.Stat(manifestPath)
	if err != nil || !manifest.Mode().IsRegular() || manifest.Size() == 0 || manifest.Size() > 4<<20 {
		return false
	}
	data, err := os.ReadFile(manifestPath)
	var parsed struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	return err == nil && json.Unmarshal(data, &parsed) == nil && parsed.Dependencies != nil
}

// Called on the window thread after the background watcher has checked disk.
func (a *app) finishPendingProjects(ready []pendingProject) {
	if a.scanning {
		return
	}
	changed := false
	for _, p := range ready {
		if _, err := os.Stat(p.file); err != nil || !pendingProjectReady(p) {
			continue
		}
		a.prefs.PinnedProjects = dedupe(append(a.prefs.PinnedProjects, p.Path))
		delete(a.prefs.HiddenProjects, localProjectKey(p.Path))
		if err := savePrefs(a.prefs); err != nil {
			logger.Printf("project creation settings: %v", err)
			continue
		}
		if !a.prefs.NoHubSync {
			entry := projectEntry{Path: p.Path, Name: filepath.Base(p.Path), Version: p.Version, Kind: p.Kind}
			added, err := exportProjects(a.hubs, []projectEntry{entry})
			if err != nil {
				logger.Printf("project creation hub export: %v", err)
				continue
			}
			if added > 0 {
				a.hubWriteAt = time.Now()
			}
		}
		if err := os.Remove(p.file); err != nil {
			logger.Printf("project creation intent: %v", err)
			continue
		}
		a.pendingProjectList = slices.DeleteFunc(a.pendingProjectList, func(item pendingProject) bool { return item.file == p.file })
		changed = true
	}
	if changed {
		a.refresh()
	}
}

// Forgetting tracking never deletes project files or stops the editor.
func (a *app) forgetPendingProject(p pendingProject) {
	if err := os.Remove(p.file); err != nil && !os.IsNotExist(err) {
		a.alert(err.Error())
		return
	}
	a.pendingProjectList = slices.DeleteFunc(a.pendingProjectList, func(item pendingProject) bool { return item.file == p.file })
}
