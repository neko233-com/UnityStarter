package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	Version        string
	DisplayVersion string
	Kind           string
	Exe            string
	Root           string
	// Simulated is an editor this app put there to try the install flow.
	Simulated bool
	Modules   []editorModule
	// ModulesCached means the list came from this app's saved official catalog.
	ModulesCached bool
	OfflineDocs   string
}

// projectEntry is a project found on disk.
type projectEntry struct {
	Path           string
	Name           string
	Version        string
	DisplayVersion string
	Kind           string
	Modified       time.Time
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
	Locale         string            `json:"locale"`
	ActiveEngine   string            `json:"activeEngine,omitempty"`
	ProjectRoots   []string          `json:"projectRoots"`
	EditorRoots    []string          `json:"editorRoots"`
	PinnedProjects []string          `json:"pinnedProjects"`
	LocalFavorites map[string]bool   `json:"localFavorites,omitempty"`
	HiddenProjects map[string]string `json:"hiddenProjects,omitempty"`
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
	return scanWithVCS(p, true)
}

// The window can show projects immediately and fill in slower VCS status
// separately. CLI consumers retain the complete synchronous scan.
func scanWithVCS(p prefs, withVCS bool) ([]editorInstall, []projectEntry, []hub) {
	hubs := detectHubs()
	roots := append([]string{}, p.EditorRoots...)
	if p.InstallRoot != "" {
		roots = append(roots, p.InstallRoot)
	} else {
		roots = append(roots, defaultInstallRoot())
	}
	roots = append(roots, legacyInstallRoots()...)
	editors := scanEditors(roots)
	projects := scanProjectsMode(p, editors, hubs, withVCS)
	versions := readTuanjieVersionMap(hubConfigDirs())
	for i := range editors {
		if editors[i].Kind == kindTuanjie {
			editors[i].DisplayVersion = versions[editors[i].Version]
		}
	}
	for i := range projects {
		if projects[i].Kind == kindTuanjie {
			projects[i].DisplayVersion = versions[projects[i].Version]
		}
	}
	return editors, projects, hubs
}

func readTuanjieVersionMap(dirs []string) map[string]string {
	versions := map[string]string{}
	for _, dir := range dirs {
		if !strings.Contains(strings.ToLower(filepath.Base(dir)), "tuanjie") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "versionMapping.json"))
		if err != nil {
			continue
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) != nil {
			continue
		}
		for key, value := range raw {
			var display string
			if json.Unmarshal(value, &display) == nil && display != "" {
				versions[key] = display
			}
		}
	}
	return versions
}

func displayVersion(version, display string) string {
	if display == "" || display == version {
		return version
	}
	return display + "  ·  " + version
}

func scanEditors(extraRoots []string) []editorInstall {
	return scanEditorsIn(append(defaultEditorRoots(), extraRoots...))
}

func scanEditorsIn(roots []string) []editorInstall {
	roots = dedupe(roots)
	var out []editorInstall
	seen := map[string]bool{}
	for _, root := range roots {
		// Users may choose either the version directory or its Editor child,
		// as well as a parent containing many version directories.
		candidateRoot := root
		if strings.EqualFold(filepath.Base(root), "Editor") {
			candidateRoot = filepath.Dir(root)
		}
		candidates := []string{candidateRoot}
		entries, err := os.ReadDir(root)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					candidates = append(candidates, filepath.Join(root, entry.Name()))
				}
			}
		}
		for _, dir := range candidates {
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
				key, err := filepath.Abs(exe)
				if err != nil {
					key = filepath.Clean(exe)
				}
				if resolved, err := filepath.EvalSymlinks(key); err == nil {
					key = resolved
				}
				if runtime.GOOS == "windows" {
					key = strings.ToLower(key)
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				kind := kindUnity
				if strings.HasPrefix(name, "Tuanjie") {
					kind = kindTuanjie
				}
				version := filepath.Base(dir)
				if actual := editorExecutableVersion(exe); actual != "" {
					version = actual
				}
				modules, cached := scannedEditorModules(dir, kind, version)
				docs := ""
				candidate := filepath.Join(dir, "Editor", "Data", "Documentation")
				if documentationFilesPresentIn(candidate, offlineDocumentationLanguage(kind)) {
					docs = candidate
				}
				out = append(out, editorInstall{Version: version, Kind: kind, Exe: exe, Root: dir, Simulated: isSimulatedEditor(dir), Modules: modules, ModulesCached: cached, OfflineDocs: docs})
			}
		}
	}
	// Keep the preferred root first when versions and engines are equal.
	sort.SliceStable(out, func(i, j int) bool {
		if c := compareVersions(out[i].Version, out[j].Version); c != 0 {
			return c > 0
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func scanProjects(p prefs, editors []editorInstall, hubs []hub) []projectEntry {
	return scanProjectsMode(p, editors, hubs, true)
}

func scanProjectsMode(p prefs, editors []editorInstall, hubs []hub, withVCS bool) []projectEntry {
	return scanProjectsWithRecent(p, editors, hubs, append(defaultProjectRoots(), p.ProjectRoots...), recentEditorProjects(), os.TempDir(), withVCS)
}

// Editor recent history also contains temporary projects made by automation.
// Ignore those automatic entries, while preserving explicit user additions,
// configured project roots, and projects listed by a Hub.
func scanProjectsWithRecent(p prefs, editors []editorInstall, hubs []hub, roots []string, recent []recentEditorProject, tempRoot string, withVCS bool) []projectEntry {
	p.PinnedProjects = append([]string(nil), p.PinnedProjects...)
	for _, entry := range recent {
		if p.LocalFavorites[localProjectKey(entry.Path)] || (!pathWithinDirectory(entry.Path, tempRoot) && !generatedProjectPath(entry.Path)) {
			p.PinnedProjects = append(p.PinnedProjects, entry.Path)
		}
	}
	return scanProjectsInMode(p, editors, hubs, roots, withVCS)
}

// A project created inside another project's generated folders is usually an
// automation workspace, not a user project. Apply this only to recent history:
// an explicit project root, pin or Hub entry still expresses user intent.
func generatedProjectPath(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for dir := filepath.Dir(filepath.Clean(path)); ; dir = filepath.Dir(dir) {
		if (strings.EqualFold(filepath.Base(dir), "Library") || strings.EqualFold(filepath.Base(dir), "Temp")) && isProject(filepath.Dir(dir)) {
			return true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return false
		}
	}
}

func pathWithinDirectory(path, root string) bool {
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return false
	}
	path, root = filepath.Clean(path), filepath.Clean(root)
	// Windows' temporary directory may use an 8.3 name while EditorPrefs
	// records the long name; resolve existing paths before comparing them.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

// scanProjectsIn reads the projects of the hubs, of the roots and of the
// projects added by hand.
func scanProjectsIn(p prefs, editors []editorInstall, hubs []hub, roots []string) []projectEntry {
	return scanProjectsInMode(p, editors, hubs, roots, true)
}

func scanProjectsInMode(p prefs, editors []editorInstall, hubs []hub, roots []string, withVCS bool) []projectEntry {
	byVersion := map[string]editorInstall{}
	for _, e := range editors {
		previous, ok := byVersion[e.Version]
		if !ok || (previous.Simulated && !e.Simulated) {
			byVersion[e.Version] = e
		}
	}

	found := map[string]projectEntry{}
	add := func(dir string, hint projectHint) {
		dir = filepath.Clean(dir)
		key := localProjectKey(dir)
		if _, hidden := p.HiddenProjects[key]; hidden {
			return
		}
		if existing, ok := found[key]; ok {
			if _, overridden := p.LocalFavorites[key]; !overridden && hint.Favorite {
				existing.Favorite = true
			}
			if hint.Hub == existing.Kind {
				existing.Hub = hint.Hub
			}
			if hint.Title != "" && existing.Name == filepath.Base(existing.Path) {
				existing.Name = hint.Title
			}
			if hint.Modified.After(existing.Modified) {
				existing.Modified = hint.Modified
			}
			found[key] = existing
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
		if modified.IsZero() && info != nil {
			modified = info.ModTime()
		}
		kind := kindOfVersion(version)
		// An installed editor of that version knows its engine best.
		if e, ok := byVersion[version]; ok {
			kind = e.Kind
		}
		vcs := vcsInfo{}
		if withVCS {
			vcs = detectVSCCached(dir)
		}
		favorite := hint.Favorite
		if local, ok := p.LocalFavorites[key]; ok {
			favorite = local
		}
		found[key] = projectEntry{
			Path:     dir,
			Name:     name,
			Version:  version,
			Kind:     kind,
			Modified: modified,
			Editor:   byVersion[version].Exe,
			Hub:      hint.Hub,
			Favorite: favorite,
			Pipeline: detectPipeline(dir),
			VCS:      vcs,
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

func localProjectKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
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
