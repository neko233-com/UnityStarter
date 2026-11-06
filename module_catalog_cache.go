package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type unityModuleCache struct {
	Schema  int            `json:"schema"`
	Version string         `json:"version"`
	Modules []editorModule `json:"modules"`
}

func unityModuleCachePath(version string) string {
	sum := sha256.Sum256([]byte(version))
	return filepath.Join(userDataDir(), "module-catalogs", "unity-"+hex.EncodeToString(sum[:])+".json")
}

func validUnityModuleCache(version string, modules []editorModule) bool {
	if version == "" || len(modules) == 0 || len(modules) > 1024 {
		return false
	}
	seen := make(map[string]bool, len(modules))
	for _, module := range modules {
		if !moduleIDPattern.MatchString(module.ID) || seen[module.ID] {
			return false
		}
		seen[module.ID] = true
	}
	return true
}

func writeUnityModuleCache(version string, modules []editorModule) error {
	if !validUnityModuleCache(version, modules) {
		return fmt.Errorf("invalid Unity module catalog")
	}
	copyModules := append([]editorModule(nil), modules...)
	for i := range copyModules {
		copyModules[i].Selected = false
	}
	data, err := json.Marshal(unityModuleCache{Schema: 1, Version: version, Modules: copyModules})
	if err != nil {
		return err
	}
	return writeFileAtomic(unityModuleCachePath(version), data)
}

func readUnityModuleCache(version string) []editorModule {
	path := unityModuleCachePath(version)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache unityModuleCache
	if json.Unmarshal(data, &cache) != nil || cache.Schema != 1 || cache.Version != version || !validUnityModuleCache(version, cache.Modules) {
		return nil
	}
	for i := range cache.Modules {
		cache.Modules[i].Selected = false
	}
	annotateSDKDependencies(cache.Modules)
	return cache.Modules
}

func scannedEditorModules(root, kind, version string) ([]editorModule, bool) {
	if modules := readEditorModules(root); len(modules) != 0 {
		return modules, false
	}
	if kind != kindUnity {
		return nil, false
	}
	modules := readUnityModuleCache(version)
	if len(modules) == 0 {
		return nil, false
	}
	applyOwnedModuleState(root, modules)
	return modules, true
}
