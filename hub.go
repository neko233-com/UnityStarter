package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// hub is a Hub installed on this machine, whose projects and favorites this
// app takes over.
//
// Unity Hub keeps its project list in projects-v1.json, which this app reads
// and writes. Tuanjie Hub keeps its list where this app cannot write it, so
// its projects are read from disk, and its favorites and folders are the
// files this app shares with it.
type hub struct {
	Kind      string
	Dir       string
	Version   string
	Exe       string
	Registry  string
	Favorites string
}

// writable reports whether the hub has a project list this app can change.
func (h hub) writable() bool { return h.Registry != "" }

var errNoRegistry = errors.New("this hub has no project list to write")

func detectHubs() []hub {
	var out []hub
	for _, dir := range hubConfigDirs() {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		h := hub{Kind: kindUnity, Dir: dir}
		if strings.Contains(strings.ToLower(filepath.Base(dir)), "tuanjie") {
			h.Kind = kindTuanjie
		}
		var info struct {
			Version        string `json:"version"`
			ExecutablePath string `json:"executablePath"`
		}
		if data, err := os.ReadFile(filepath.Join(dir, "hubInfo.json")); err == nil {
			if json.Unmarshal(data, &info) == nil {
				h.Version, h.Exe = info.Version, info.ExecutablePath
			}
		}
		registry := filepath.Join(dir, "projects-v1.json")
		if _, err := os.Stat(registry); err == nil {
			h.Registry = registry
		}
		h.Favorites = filepath.Join(dir, "favoriteProjects.json")
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// hubProject is a project a hub lists.
type hubProject struct {
	Path     string
	Name     string
	Version  string
	Modified time.Time
	Favorite bool
}

// projects reads what the hub lists: its project registry, and the favorites
// of a hub without one.
func (h hub) projects() map[string]hubProject {
	out := map[string]hubProject{}
	doc, data := readRegistry(h.Registry)
	_ = doc
	for path, raw := range data {
		var entry struct {
			Title        string `json:"title"`
			Path         string `json:"path"`
			Version      string `json:"version"`
			LastModified int64  `json:"lastModified"`
			Favorite     bool   `json:"isFavorite"`
		}
		_ = json.Unmarshal(raw, &entry)
		if entry.Path != "" {
			path = entry.Path
		}
		p := hubProject{Path: filepath.Clean(path), Name: entry.Title, Version: entry.Version, Favorite: entry.Favorite}
		if entry.LastModified > 0 {
			p.Modified = time.UnixMilli(entry.LastModified)
		}
		out[p.Path] = p
	}
	for _, path := range h.favorites() {
		p := out[path]
		p.Path = path
		p.Favorite = true
		out[path] = p
	}
	return out
}

// addProject adds a project to the hub's list, with the fields Unity Hub
// writes for the projects it adds.
func (h hub) addProject(p projectEntry) error {
	if !h.writable() {
		return errNoRegistry
	}
	doc, data := readRegistry(h.Registry)
	if _, ok := data[p.Path]; ok {
		return nil
	}
	entry := map[string]any{
		"title":                p.Name,
		"lastModified":         time.Now().UnixMilli(),
		"isCustomEditor":       false,
		"path":                 p.Path,
		"containingFolderPath": filepath.Dir(p.Path),
		"version":              p.Version,
		"architecture":         "x86_64",
		"changeset":            "",
		"isFavorite":           p.Favorite,
		"cloudProjectId":       nil,
		"organizationId":       nil,
		"localProjectId":       randomID(),
		"cloudEnabled":         true,
		"projectName":          nil,
		"hasCustomDisplayName": false,
		"renderPipeline":       nil,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data[p.Path] = raw
	return writeRegistry(h.Registry, doc, data)
}

// setFavorite stars or unstars a project, in the hub's favorites and in its
// registry.
func (h hub) setFavorite(path string, on bool) error {
	paths := h.favorites()
	next := make([]string, 0, len(paths)+1)
	for _, p := range paths {
		if p != path {
			next = append(next, p)
		}
	}
	if on {
		next = append(next, path)
	}
	sort.Strings(next)
	if err := writeFavorites(h.Favorites, next); err != nil {
		return err
	}
	if h.writable() {
		doc, data := readRegistry(h.Registry)
		if raw, ok := data[path]; ok {
			var entry map[string]any
			if json.Unmarshal(raw, &entry) == nil {
				entry["isFavorite"] = on
				if b, err := json.Marshal(entry); err == nil {
					data[path] = b
					return writeRegistry(h.Registry, doc, data)
				}
			}
		}
	}
	return nil
}

// favorites reads favoriteProjects.json, which holds a JSON string of a JSON
// array of paths.
func (h hub) favorites() []string {
	data, err := os.ReadFile(h.Favorites)
	if err != nil {
		return nil
	}
	var encoded string
	if json.Unmarshal(data, &encoded) == nil {
		var paths []string
		if json.Unmarshal([]byte(encoded), &paths) == nil {
			return cleanPaths(paths)
		}
	}
	var paths []string
	if json.Unmarshal(data, &paths) == nil {
		return cleanPaths(paths)
	}
	return nil
}

func writeFavorites(path string, paths []string) error {
	if paths == nil {
		paths = []string{}
	}
	inner, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(string(inner))
	if err != nil {
		return err
	}
	return writeFileAtomic(path, encoded)
}

// readRegistry reads projects-v1.json, keeping the keys this app does not
// know.
func readRegistry(path string) (doc map[string]json.RawMessage, data map[string]json.RawMessage) {
	doc = map[string]json.RawMessage{}
	data = map[string]json.RawMessage{}
	if path == "" {
		return doc, data
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return doc, data
	}
	if json.Unmarshal(raw, &doc) != nil {
		return map[string]json.RawMessage{}, data
	}
	if toplevel, ok := doc["data"]; ok {
		_ = json.Unmarshal(toplevel, &data)
	}
	if data == nil {
		data = map[string]json.RawMessage{}
	}
	return doc, data
}

func writeRegistry(path string, doc map[string]json.RawMessage, data map[string]json.RawMessage) error {
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	rawData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	doc["data"] = rawData
	if _, ok := doc["schema_version"]; !ok {
		doc["schema_version"] = json.RawMessage(`"v1"`)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// writeFileAtomic replaces a file in one step, so that a hub reading it never
// sees half a file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".unity-starter-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// lists reports whether the hub's project list already holds a path.
func (h hub) lists(path string) bool {
	_, data := readRegistry(h.Registry)
	_, ok := data[path]
	return ok
}

// exportProjects adds projects to the hubs that own their engine, where those
// hubs keep a list this app writes. It returns how many it added.
func exportProjects(hubs []hub, projects []projectEntry) (int, error) {
	added := 0
	for _, p := range projects {
		if p.Hub != "" {
			continue // a hub already lists it
		}
		for _, h := range hubs {
			if !h.writable() || h.Kind != p.Kind || h.lists(p.Path) {
				continue
			}
			if err := h.addProject(p); err != nil {
				return added, err
			}
			added++
			logger.Printf("hub: added %s to %s", p.Path, h.Kind)
		}
	}
	return added, nil
}

// hubFiles are the files this app shares with the hubs, which tell when a
// hub changes what it lists.
var hubFiles = map[string]bool{
	"projects-v1.json":      true,
	"favoriteProjects.json": true,
}

// watchHubs calls onChange when a hub changes the files this app shares with
// it, so that what the hub lists shows here while both run.
func watchHubs(ctx context.Context, hubs []hub, onChange func()) {
	for _, h := range hubs {
		watcher, err := mygo.WatchFiles(ctx, h.Dir, mygo.FileWatchOptions{})
		if err != nil {
			continue
		}
		go func(w *mygo.FileWatcher) {
			defer w.Close()
			for {
				events, err := w.Next(ctx)
				if err != nil {
					return
				}
				for _, e := range events {
					if hubFiles[filepath.Base(e.Path)] {
						onChange()
						break
					}
				}
			}
		}(watcher)
	}
}
