package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var unityModuleCatalogURL = "https://services.api.unity.com/unity/editor/release/v1/releases?limit=1&version="

type unityModule struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	Integrity    string            `json:"integrity"`
	Destination  string            `json:"destination"`
	Command      string            `json:"cmd"`
	Hidden       bool              `json:"hidden"`
	EULA         []json.RawMessage `json:"eula"`
	Children     []unityModule     `json:"subModules"`
	DownloadSize struct {
		Value int64 `json:"value"`
	} `json:"downloadSize"`
}

func appendUnityModules(out *[]editorModule, modules []unityModule, parent string) {
	for _, m := range modules {
		checksum := ""
		if strings.HasPrefix(m.Integrity, "md5-") {
			if b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(m.Integrity, "md5-")); err == nil && len(b) == 16 {
				checksum = hex.EncodeToString(b)
			}
		}
		eula := ""
		if len(m.EULA) > 0 {
			eula = "required"
		}
		*out = append(*out, editorModule{ID: m.ID, Name: m.Name, URL: m.URL, Checksum: checksum, Destination: m.Destination, Command: m.Command, Parent: parent, EULA: eula, Visible: !m.Hidden, DownloadSize: m.DownloadSize.Value})
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
	return modules, nil
}

func cachedTuanjieModules(version string, dirs []string) []editorModule {
	for _, dir := range dirs {
		if !strings.Contains(strings.ToLower(filepath.Base(dir)), "tuanjie") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "releases.json"))
		if err != nil {
			continue
		}
		var file struct {
			Official []struct {
				Version string         `json:"version"`
				Modules []editorModule `json:"modules"`
			} `json:"official"`
			Beta []struct {
				Version string         `json:"version"`
				Modules []editorModule `json:"modules"`
			} `json:"beta"`
		}
		if json.Unmarshal(data, &file) != nil {
			continue
		}
		for _, release := range append(file.Official, file.Beta...) {
			if release.Version == version {
				return release.Modules
			}
		}
	}
	return nil
}

func moduleStatePath(root string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(userDataDir(), "modules", hex.EncodeToString(digest[:])+".json")
}

func readModuleState(root string) map[string]bool {
	state := map[string]bool{}
	if data, err := os.ReadFile(moduleStatePath(root)); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	return state
}

func markOwnedModuleInstalled(root, id string) error {
	state := readModuleState(root)
	state[id] = true
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeFileAtomic(moduleStatePath(root), data)
}

func moduleCatalog(ctx context.Context, e editorInstall) ([]editorModule, error) {
	var modules []editorModule
	var err error
	if e.Kind == kindTuanjie {
		modules = cachedTuanjieModules(e.Version, hubConfigDirs())
		if len(modules) == 0 {
			return nil, fmt.Errorf("no cached modules for this editor")
		}
	} else {
		modules, err = fetchUnityModules(ctx, e.Version)
		if err != nil {
			return nil, err
		}
	}
	state := readModuleState(e.Root)
	for i := range modules {
		if state[modules[i].ID] && moduleOutput(modules[i]) != "" {
			if info, err := os.Stat(filepath.Join(e.Root, "Editor", "Data", "PlaybackEngines", moduleOutput(modules[i]))); err == nil && info.IsDir() {
				modules[i].Selected = true
			}
		}
	}
	return modules, nil
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
		modules, err := moduleCatalog(ctx, e)
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
		})
	}()
}
