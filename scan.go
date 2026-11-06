package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// The two engines this app manages. A project and an editor both belong to
// one of them: Unity (international) or Tuanjie (团结引擎).
const (
	kindUnity   = "unity"
	kindTuanjie = "tuanjie"
)

// editorInstall is an editor installed on this machine.
type editorInstall struct {
	Version string
	Kind    string
	Exe     string
	Root    string
	// Simulated is an editor this app put there to try the install flow.
	Simulated bool
}

// projectEntry is a project found on disk.
type projectEntry struct {
	Path     string
	Name     string
	Version  string
	Kind     string
	Modified time.Time
	// Editor is the executable of the matching editor, empty when its
	// version is not installed.
	Editor string
	// Hub is the hub that lists the project, empty when it was only found
	// on disk.
	Hub string
	// Favorite is the hub's star.
	Favorite bool
	// Pipeline is the render pipeline the project's packages name.
	Pipeline string
	// VCS is the version control the project is kept in.
	VCS vcsInfo
}

// prefs is what the app keeps between launches.
type prefs struct {
	Locale         string   `json:"locale"`
	ProjectRoots   []string `json:"projectRoots"`
	EditorRoots    []string `json:"editorRoots"`
	PinnedProjects []string `json:"pinnedProjects"`
	// InstallRoot is where downloaded editors are installed, a folder per
	// version.
	InstallRoot string `json:"installRoot"`
	// DemoMode installs stand-in editors instead of downloading, so that the
	// flow can be tried without the gigabytes.
	DemoMode bool `json:"demoMode"`
	// NoHubSync leaves the hubs' files as they are, only reading them.
	NoHubSync bool `json:"noHubSync"`
}

// userDataDir is where the app keeps its settings, whether the window runs or
// the command line does.
func userDataDir() string {
	if mygo.App.IsReady() {
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			return dir
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, appName)
	}
	return "."
}

func prefsPath() string {
	return filepath.Join(userDataDir(), "settings.json")
}

// legacyPrefsDirs are the data folders of earlier names of the app, whose
// settings are read when there are none of this name.
var legacyPrefsDirs = []string{"Unity Starter", "unitystarter.test", "UnityStarter Dev"}

func loadPrefs() prefs {
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		if legacy, ok := readLegacyPrefs(); ok {
			data = legacy
			logger.Printf("settings: read the settings of an earlier name")
		}
	}
	var p prefs
	if data != nil {
		_ = json.Unmarshal(data, &p)
	}
	return p
}

func loadPrefsFrom(path string) prefs {
	var p prefs
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	return p
}

func readLegacyPrefs() ([]byte, bool) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, false
	}
	for _, name := range legacyPrefsDirs {
		if data, err := os.ReadFile(filepath.Join(base, name, "settings.json")); err == nil {
			return data, true
		}
	}
	return nil, false
}

func savePrefs(p prefs) error {
	return savePrefsTo(prefsPath(), p)
}

func savePrefsTo(path string, p prefs) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// scan reads the hubs, the editors and the projects of this machine.
func scan(p prefs) ([]editorInstall, []projectEntry, []hub) {
	hubs := detectHubs()
	roots := append([]string{}, p.EditorRoots...)
	if p.InstallRoot != "" {
		roots = append(roots, p.InstallRoot)
	} else {
		roots = append(roots, defaultInstallRoot())
	}
	roots = append(roots, legacyInstallRoots()...)
	editors := scanEditors(roots)
	return editors, scanProjects(p, editors, hubs), hubs
}

func scanEditors(extraRoots []string) []editorInstall {
	roots := dedupe(append(defaultEditorRoots(), extraRoots...))
	var out []editorInstall
	seen := map[string]bool{}
	// One entry per version, as the hubs list them, preferring the first
	// folder that has it.
	seenVersion := map[string]bool{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			version := entry.Name()
			dir := filepath.Join(root, version)
			if _, err := os.Stat(filepath.Join(dir, installMarker)); err == nil {
				continue
			}
			for _, name := range []string{"Unity.exe", "Tuanjie.exe", "Unity", "Tuanjie"} {
				exe := filepath.Join(dir, "Editor", name)
				if name == "Unity" || name == "Tuanjie" {
					// macOS bundles the editor in an app.
					exe = filepath.Join(dir, "Editor", name+".app", "Contents", "MacOS", name)
				}
				info, err := os.Stat(exe)
				if err != nil || info.IsDir() {
					continue
				}
				key := strings.ToLower(exe)
				if seen[key] {
					continue
				}
				seen[key] = true
				kind := kindUnity
				if strings.HasPrefix(name, "Tuanjie") {
					kind = kindTuanjie
				}
				if seenVersion[kind+" "+version] {
					continue
				}
				seenVersion[kind+" "+version] = true
				out = append(out, editorInstall{Version: version, Kind: kind, Exe: exe, Root: dir, Simulated: isSimulatedEditor(dir)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if c := compareVersions(out[i].Version, out[j].Version); c != 0 {
			return c > 0
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func scanProjects(p prefs, editors []editorInstall, hubs []hub) []projectEntry {
	return scanProjectsIn(p, editors, hubs, append(defaultProjectRoots(), p.ProjectRoots...))
}

// scanProjectsIn reads the projects of the hubs, of the roots and of the
// projects added by hand.
func scanProjectsIn(p prefs, editors []editorInstall, hubs []hub, roots []string) []projectEntry {
	byVersion := map[string]string{}
	kindByVersion := map[string]string{}
	for _, e := range editors {
		if _, ok := byVersion[e.Version]; !ok {
			byVersion[e.Version] = e.Exe
		}
		if _, ok := kindByVersion[e.Version]; !ok {
			kindByVersion[e.Version] = e.Kind
		}
	}

	found := map[string]projectEntry{}
	add := func(dir string, hint projectHint) {
		dir = filepath.Clean(dir)
		if _, ok := found[dir]; ok {
			return
		}
		version, info, ok := readProject(dir)
		if !ok {
			return
		}
		name := filepath.Base(dir)
		if hint.Title != "" {
			name = hint.Title
		}
		modified := hint.Modified
		if modified.IsZero() {
			modified = info.ModTime()
		}
		kind := kindOfVersion(version)
		// An installed editor of that version knows its engine best.
		if k, ok := kindByVersion[version]; ok {
			kind = k
		}
		found[dir] = projectEntry{
			Path:     dir,
			Name:     name,
			Version:  version,
			Kind:     kind,
			Modified: modified,
			Editor:   byVersion[version],
			Hub:      hint.Hub,
			Favorite: hint.Favorite,
			Pipeline: detectPipeline(dir),
			VCS:      detectVSCCached(dir),
		}
	}

	for dir, hint := range hubHints(hubs) {
		add(dir, hint)
	}
	for _, root := range dedupe(roots) {
		walkProjects(root, 4, add)
	}
	for _, dir := range p.PinnedProjects {
		add(dir, projectHint{})
	}

	out := make([]projectEntry, 0, len(found))
	for _, p := range found {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Modified.Equal(out[j].Modified) {
			return out[i].Modified.After(out[j].Modified)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// projectHint is what a hub knows about a project before it is read.
type projectHint struct {
	Title    string
	Version  string
	Modified time.Time
	Hub      string
	Favorite bool
}

// hubHints is what every hub lists, by project path.
func hubHints(hubs []hub) map[string]projectHint {
	out := map[string]projectHint{}
	for _, h := range hubs {
		for path, project := range h.projects() {
			hint := out[path]
			hint.Hub = h.Kind
			if project.Name != "" {
				hint.Title = project.Name
			}
			if !project.Modified.IsZero() {
				hint.Modified = project.Modified
			}
			hint.Favorite = hint.Favorite || project.Favorite
			out[path] = hint
		}
	}
	return out
}

func cleanPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, filepath.Clean(p))
	}
	return out
}

func readProject(dir string) (string, os.FileInfo, bool) {
	path := filepath.Join(dir, "ProjectSettings", "ProjectVersion.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, false
	}
	version := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "m_EditorVersion:"); ok {
			version = strings.TrimSpace(v)
			break
		}
	}
	if version == "" {
		return "", nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return version, nil, true
	}
	return version, info, true
}

// walkProjects calls add for every project under root, up to maxDepth
// directories deep.
func walkProjects(root string, maxDepth int, add func(string, projectHint)) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	root = filepath.Clean(root)
	if isProject(root) {
		add(root, projectHint{})
		return
	}
	baseDepth := strings.Count(root, string(os.PathSeparator))
	visited := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		visited++
		if visited > 20000 {
			return filepath.SkipAll
		}
		if path == root {
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - baseDepth
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if skipProjectDir(d.Name()) {
			return filepath.SkipDir
		}
		if isProject(path) {
			add(path, projectHint{})
			return filepath.SkipDir
		}
		return nil
	})
}

func isProject(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "ProjectSettings", "ProjectVersion.txt"))
	return err == nil && !info.IsDir()
}

func skipProjectDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "library", "temp", "obj", "logs", "build", "builds", "node_modules", "packages", "assets", "userdata", "recordings":
		return true
	}
	return false
}

// kindOfVersion reads the engine from a project's editor version: Tuanjie
// builds carry a "t" build suffix, as 2022.3.62t17, where Unity builds carry
// an "f" one, as 2022.3.51f1.
func kindOfVersion(version string) string {
	v := strings.ToLower(version)
	if i := strings.IndexByte(v, 't'); i > 0 {
		if j := strings.IndexByte(v, 'f'); j < 0 || i < j {
			return kindTuanjie
		}
	}
	return kindUnity
}

var editorVersionPattern = regexp.MustCompile(`(?i)^(\d+)\.(\d+)\.(\d+)(?:([a-z]+)(\d+))?(.*)$`)

// compareVersions orders the three version components, release channel, then
// the channel revision; a final f1 is newer than a beta b99 of the same base.
func compareVersions(a, b string) int {
	am, bm := editorVersionPattern.FindStringSubmatch(a), editorVersionPattern.FindStringSubmatch(b)
	if am != nil && bm != nil {
		for i := 1; i <= 3; i++ {
			x, _ := strconv.Atoi(am[i])
			y, _ := strconv.Atoi(bm[i])
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
		}
		rank := map[string]int{"a": 0, "b": 1, "f": 2, "t": 2, "p": 3, "": 4}
		achannel, bchannel := strings.ToLower(am[4]), strings.ToLower(bm[4])
		x, ok := rank[achannel]
		if !ok {
			x = -1
		}
		y, ok := rank[bchannel]
		if !ok {
			y = -1
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		if achannel != bchannel {
			return strings.Compare(achannel, bchannel)
		}
		x, _ = strconv.Atoi(am[5])
		y, _ = strconv.Atoi(bm[5])
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	}
	as, bs := versionParts(a), versionParts(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(a, b)
}

func versionParts(v string) []int {
	var parts []int
	for _, field := range strings.FieldsFunc(v, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if n, err := strconv.Atoi(field); err == nil {
			parts = append(parts, n)
		}
	}
	return parts
}

func dedupe(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}
