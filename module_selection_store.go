package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type savedModuleSelection struct {
	Schema  int      `json:"schema"`
	Editor  string   `json:"editor"`
	Version string   `json:"version"`
	Engine  string   `json:"engine"`
	Modules []string `json:"modules"`
}

func moduleSelectionFile(e editorInstall) (string, error) {
	if !filepath.IsAbs(e.Exe) || e.Version == "" || (e.Kind != kindUnity && e.Kind != kindTuanjie) {
		return "", fmt.Errorf("invalid editor identity")
	}
	sum := sha256.Sum256([]byte(localProjectKey(e.Exe)))
	return filepath.Join(userDataDir(), "module-selections", hex.EncodeToString(sum[:])+".json"), nil
}

func saveModuleSelection(e editorInstall, selected map[string]bool) error {
	path, err := moduleSelectionFile(e)
	if err != nil {
		return err
	}
	var ids []string
	for id, on := range selected {
		if !on {
			continue
		}
		if !moduleIDPattern.MatchString(id) {
			return fmt.Errorf("invalid module selection")
		}
		ids = append(ids, id)
	}
	if len(ids) > 256 {
		return fmt.Errorf("too many selected modules")
	}
	if len(ids) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	sort.Strings(ids)
	record := savedModuleSelection{Schema: 1, Editor: filepath.Clean(e.Exe), Version: e.Version, Engine: e.Kind, Modules: ids}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func loadModuleSelection(e editorInstall) (map[string]bool, error) {
	path, err := moduleSelectionFile(e)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64<<10 {
		return nil, fmt.Errorf("invalid module selection file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("module selection file changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, fmt.Errorf("invalid module selection file")
	}
	var record savedModuleSelection
	if json.Unmarshal(data, &record) != nil || record.Schema != 1 || len(record.Modules) > 256 {
		return nil, fmt.Errorf("invalid module selection file")
	}
	if localProjectKey(record.Editor) != localProjectKey(e.Exe) || record.Version != e.Version || record.Engine != e.Kind {
		return nil, nil
	}
	selection := make(map[string]bool, len(record.Modules))
	for _, id := range record.Modules {
		if !moduleIDPattern.MatchString(id) || selection[id] {
			return nil, fmt.Errorf("invalid module selection file")
		}
		selection[id] = true
	}
	return selection, nil
}

// UI state changes happen on the main thread. Tests may inject an isolated
// store; a Tester without a window otherwise remains in-memory only.
func (a *app) persistModuleSelection(e editorInstall) error {
	if a.moduleSelectionStore != nil {
		return a.moduleSelectionStore(e, a.moduleSelection)
	}
	if a.win == nil {
		return nil
	}
	return saveModuleSelection(e, a.moduleSelection)
}

func (a *app) restoreModuleSelection(e editorInstall) (map[string]bool, error) {
	if a.moduleSelectionLoad != nil {
		return a.moduleSelectionLoad(e)
	}
	if a.win == nil {
		return nil, nil
	}
	return loadModuleSelection(e)
}
