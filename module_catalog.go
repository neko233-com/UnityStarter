package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"unitystarter/locales"
)

var unityModuleCatalogURL = "https://services.api.unity.com/unity/editor/release/v1/releases?limit=1&version="

type unityModule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Integrity   string `json:"integrity"`
	Destination string `json:"destination"`
	Command     string `json:"cmd"`
	Hidden      bool   `json:"hidden"`
	Required    bool   `json:"required"`
	EULA        []struct {
		URL string `json:"url"`
	} `json:"eula"`
	Rename struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"extractedPathRename"`
	Children     []unityModule `json:"subModules"`
	DownloadSize struct {
		Value int64 `json:"value"`
	} `json:"downloadSize"`
}

func appendUnityModules(out *[]editorModule, modules []unityModule, parent string) {
	for _, m := range modules {
		checksum := ""
		if strings.HasPrefix(m.Integrity, "md5-") {
			if _, b, err := integrityHasher(m.Integrity); err == nil && len(b) == 16 {
				checksum = hex.EncodeToString(b)
			}
		}
		eula := ""
		if len(m.EULA) == 1 {
			eula = m.EULA[0].URL
		} else if len(m.EULA) > 1 {
			eula = "multiple"
		}
		*out = append(*out, editorModule{ID: m.ID, Name: m.Name, URL: m.URL, Checksum: checksum, Integrity: m.Integrity, Destination: m.Destination, Command: m.Command, Parent: parent, Required: m.Required, RenameFrom: m.Rename.From, RenameTo: m.Rename.To, EULA: eula, Visible: !m.Hidden, DownloadSize: m.DownloadSize.Value})
		appendUnityModules(out, m.Children, m.ID)
	}
}

func fetchUnityModules(ctx context.Context, version string) ([]editorModule, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, unityModuleCatalogURL+url.QueryEscape(version), nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("official module catalog unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official module catalog: %s", resp.Status)
	}
	var body struct {
		Results []struct {
			Version   string `json:"version"`
			Downloads []struct {
				Platform     string        `json:"platform"`
				Architecture string        `json:"architecture"`
				Modules      []unityModule `json:"modules"`
			} `json:"downloads"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&body); err != nil {
		return nil, err
	}
	var modules []editorModule
	for _, release := range body.Results {
		if release.Version != version {
			continue
		}
		for _, d := range release.Downloads {
			if d.Platform == "WINDOWS" && d.Architecture == "X86_64" {
				appendUnityModules(&modules, d.Modules, "")
				break
			}
		}
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no modules for this editor")
	}
	annotateSDKDependencies(modules)
	return modules, nil
}

func annotateSDKDependencies(modules []editorModule) {
	type dependency struct {
		ID, URL, Integrity, Checksum, Destination, RenameFrom, RenameTo, EULA, EULA2, Command string
		DownloadSize                                                                          int64
	}
	for i := range modules {
		if modules[i].ID != "android-sdk-ndk-tools" {
			continue
		}
		modules[i].DependencyFingerprint = ""
		var children []dependency
		for _, child := range modules {
			if child.Parent == modules[i].ID && child.Required {
				children = append(children, dependency{child.ID, child.URL, child.Integrity, child.Checksum, child.Destination, child.RenameFrom, child.RenameTo, child.EULA, child.EULA2, child.Command, child.DownloadSize})
			}
		}
		if len(children) == 0 {
			continue
		}
		sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
		data, err := json.Marshal(children)
		if err == nil {
			sum := sha256.Sum256(data)
			modules[i].DependencyFingerprint = hex.EncodeToString(sum[:])
		}
	}
}

func cachedTuanjieModules(version string, dirs []string) []editorModule {
	if data, err := os.ReadFile(tuanjieReleasesCachePath()); err == nil {
		if modules := tuanjieModulesFromCatalog(data, version); len(modules) > 0 {
			return modules
		}
	}
	for _, dir := range dirs {
		if !strings.Contains(strings.ToLower(filepath.Base(dir)), "tuanjie") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "releases.json"))
		if err != nil {
			continue
		}
		if modules := tuanjieModulesFromCatalog(data, version); len(modules) > 0 {
			return modules
		}
	}
	return nil
}

func tuanjieModulesFromCatalog(data []byte, version string) []editorModule {
	var file tuanjieCatalog
	if json.Unmarshal(data, &file) != nil {
		return nil
	}
	for _, release := range append(file.Official, file.Beta...) {
		if release.Version == version {
			return release.Modules
		}
	}
	return nil
}

func moduleStatePath(root string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(userDataDir(), "modules", hex.EncodeToString(digest[:])+".json")
}

type ownedModuleState struct {
	Installed   bool   `json:"installed"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func moduleFingerprint(m editorModule) string {
	value := m.ID + "\x00" + m.URL + "\x00" + m.Checksum + "\x00" + m.Destination
	if m.DependencyFingerprint != "" {
		value += "\x00" + m.DependencyFingerprint
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func readModuleState(root string) map[string]ownedModuleState {
	state := map[string]ownedModuleState{}
	var raw map[string]json.RawMessage
	if data, err := os.ReadFile(moduleStatePath(root)); err != nil || json.Unmarshal(data, &raw) != nil {
		return state
	}
	for id, value := range raw {
		var record ownedModuleState
		if json.Unmarshal(value, &record) == nil {
			state[id] = record
			continue
		}
		var legacy bool
		if json.Unmarshal(value, &legacy) == nil && legacy {
			state[id] = ownedModuleState{Installed: true}
		}
	}
	return state
}

func moduleStateMatches(state map[string]ownedModuleState, m editorModule) bool {
	record, ok := state[m.ID]
	return ok && record.Installed && (record.Fingerprint == "" || record.Fingerprint == moduleFingerprint(m))
}

func markOwnedModuleInstalled(root string, module editorModule) error {
	state := readModuleState(root)
	state[module.ID] = ownedModuleState{Installed: true, Fingerprint: moduleFingerprint(module)}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeFileAtomic(moduleStatePath(root), data)
}

func moduleCatalog(ctx context.Context, e editorInstall) ([]editorModule, error) {
	modules, _, err := moduleCatalogWithSource(ctx, e)
	return modules, err
}

func moduleCatalogWithSource(ctx context.Context, e editorInstall) ([]editorModule, bool, error) {
	var modules []editorModule
	var err error
	fromCache := false
	if e.Kind == kindTuanjie {
		modules = cachedTuanjieModules(e.Version, hubConfigDirs())
		if len(modules) == 0 {
			data, fetchErr := fetchTuanjieCatalog(ctx)
			if fetchErr != nil {
				return nil, false, fetchErr
			}
			if cacheErr := writeFileAtomic(tuanjieReleasesCachePath(), data); cacheErr != nil {
				logger.Printf("Tuanjie catalog cache: %v", cacheErr)
			}
			modules = tuanjieModulesFromCatalog(data, e.Version)
			if len(modules) == 0 {
				return nil, false, fmt.Errorf("no modules for this editor in the official catalog")
			}
		}
	} else {
		modules, err = fetchUnityModules(ctx, e.Version)
		if err != nil {
			modules = readUnityModuleCache(e.Version)
			if len(modules) == 0 {
				return nil, false, err
			}
			fromCache = true
		} else if cacheErr := writeUnityModuleCache(e.Version, modules); cacheErr != nil {
			logger.Printf("Unity module catalog cache: %v", cacheErr)
		}
	}
	applyOwnedModuleState(e.Root, modules)
	return modules, fromCache, nil
}

func applyOwnedModuleState(root string, modules []editorModule) {
	state := readModuleState(root)
	for i := range modules {
		if moduleStateMatches(state, modules[i]) && moduleFilesPresent(root, modules[i]) {
			modules[i].Selected = true
		}
	}
}

func (a *app) loadEditorModules(e editorInstall) {
	if a.win == nil || a.moduleLoading {
		return
	}
	a.moduleLoading = true
	a.moduleStatus = ""
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		modules := readEditorModules(e.Root)
		cached := false
		var err error
		if len(modules) == 0 {
			modules, cached, err = moduleCatalogWithSource(ctx, e)
		}
		win.Update(func() {
			a.moduleLoading = false
			if a.manageEditor.Exe != e.Exe {
				return
			}
			if err != nil {
				a.moduleStatus = err.Error()
				return
			}
			a.manageEditor.Modules = modules
			a.manageEditor.ModulesCached = cached
			if cached {
				a.moduleStatus = locales.InstallModuleCached()
			}
			if e.Kind == kindUnity {
				a.refresh()
			}
		})
	}()
}
