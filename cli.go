package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/egoist/mygo/i18n"

	"unitystarter/locales"
)

// scanResult is what the machine has, as the command line lists it.
type scanResult struct {
	editors  []editorInstall
	projects []projectEntry
	hubs     []hub
}

func scanAll(p prefs) scanResult {
	editors, projects, hubs := scan(p)
	return scanResult{editors: editors, projects: projects, hubs: hubs}
}

type cliOptions struct {
	json     bool
	lang     string
	query    string
	editor   string
	verbose  bool
	simulate bool
}

// runCLI runs one command without a window, and returns the exit code.
func runCLI(argv []string, out io.Writer, scanFn func(prefs) scanResult) int {
	opts, args, err := parseCLI(argv)
	if err != nil {
		fmt.Fprintln(out, err)
		return 2
	}
	startConsoleLogging(opts.verbose)
	if opts.lang != "" {
		tag := opts.lang
		i18n.SetLocaleFunc(func() string { return tag })
	}

	command := "help"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	logger.Printf("cli: %v", argv)
	switch command {
	case "help":
		fmt.Fprintln(out, locales.CliHelp())
		return 0
	case "version":
		fmt.Fprintln(out, locales.CliVersion(appVersion))
		return 0
	case "projects":
		return cliProjects(opts, scanFn, out)
	case "editors":
		return cliEditors(opts, scanFn, out)
	case "open":
		return cliOpen(args, scanFn, out)
	case "new":
		return cliNew(opts, args, scanFn, out)
	case "hub":
		return cliHub(opts, args, scanFn, out)
	case "releases":
		return cliReleases(opts, scanFn, out)
	case "install":
		return cliInstall(opts, args, out)
	case "update":
		return cliUpdate(opts, out)
	default:
		fmt.Fprintf(out, "%s\n%s\n", locales.CliUnknownCommand(command), locales.CliUsageHint())
		return 2
	}
}

func parseCLI(argv []string) (cliOptions, []string, error) {
	var opts cliOptions
	var args []string
	for i := 0; i < len(argv); i++ {
		switch arg := argv[i]; {
		case arg == "--json":
			opts.json = true
		case arg == "--verbose":
			opts.verbose = true
		case arg == "--simulate":
			opts.simulate = true
		case arg == "--version":
			args = append(args, "version")
		case arg == "--lang":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--lang needs a language")
			}
			i++
			opts.lang = argv[i]
		case strings.HasPrefix(arg, "--lang="):
			opts.lang = strings.TrimPrefix(arg, "--lang=")
		case arg == "--query" || arg == "-q":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--query needs text")
			}
			i++
			opts.query = argv[i]
		case strings.HasPrefix(arg, "--query="):
			opts.query = strings.TrimPrefix(arg, "--query=")
		case arg == "--editor":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--editor needs a version")
			}
			i++
			opts.editor = argv[i]
		case strings.HasPrefix(arg, "--editor="):
			opts.editor = strings.TrimPrefix(arg, "--editor=")
		case arg == "-h":
			args = append(args, "help")
		case strings.HasPrefix(arg, "-"):
			return opts, nil, fmt.Errorf("unknown flag: %s", arg)
		default:
			args = append(args, arg)
		}
	}
	return opts, args, nil
}

type projectJSON struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Version  string `json:"version"`
	Engine   string `json:"engine"`
	Hub      string `json:"hub,omitempty"`
	Favorite bool   `json:"favorite"`
	Editor   string `json:"editor,omitempty"`
	Modified int64  `json:"modified,omitempty"`
}

type editorJSON struct {
	Version string `json:"version"`
	Engine  string `json:"engine"`
	Exe     string `json:"exe"`
}

type hubJSON struct {
	Engine  string `json:"engine"`
	Version string `json:"version"`
	Dir     string `json:"dir"`
	Mode    string `json:"mode"`
}

func cliProjects(opts cliOptions, scanFn func(prefs) scanResult, out io.Writer) int {
	res := scanFn(loadPrefs())
	items := filterProjects(res.projects, opts.query)
	if opts.json {
		list := make([]projectJSON, 0, len(items))
		for _, p := range items {
			list = append(list, projectJSON{
				Name:     p.Name,
				Path:     p.Path,
				Version:  p.Version,
				Engine:   p.Kind,
				Hub:      p.Hub,
				Favorite: p.Favorite,
				Editor:   p.Editor,
				Modified: p.Modified.UnixMilli(),
			})
		}
		return writeJSON(out, list)
	}
	if len(items) == 0 {
		fmt.Fprintln(out, locales.CliNoProjects())
		return 0
	}
	fmt.Fprintln(out, locales.CliProjectsHeading())
	cols := table(out)
	for _, p := range items {
		star := ""
		if p.Favorite {
			star = "*"
		}
		fmt.Fprintf(cols, "%s\t%s\t%s\t%s\n", p.Version, channelName(p.Kind), star, p.Path)
	}
	cols.Flush()
	return 0
}

func cliEditors(opts cliOptions, scanFn func(prefs) scanResult, out io.Writer) int {
	res := scanFn(loadPrefs())
	if opts.json {
		list := make([]editorJSON, 0, len(res.editors))
		for _, e := range res.editors {
			list = append(list, editorJSON{Version: e.Version, Engine: e.Kind, Exe: e.Exe})
		}
		return writeJSON(out, list)
	}
	if len(res.editors) == 0 {
		fmt.Fprintln(out, locales.CliNoEditors())
		return 0
	}
	fmt.Fprintln(out, locales.CliEditorsHeading())
	cols := table(out)
	for _, e := range res.editors {
		fmt.Fprintf(cols, "%s\t%s\t%s\n", e.Version, channelName(e.Kind), e.Exe)
	}
	cols.Flush()
	return 0
}

func cliOpen(args []string, scanFn func(prefs) scanResult, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, locales.CliUsageHint())
		return 2
	}
	target := args[0]
	res := scanFn(loadPrefs())
	p, message, ok := findProject(res.projects, target)
	if !ok {
		fmt.Fprintln(out, message)
		return 1
	}
	if p.Editor == "" {
		fmt.Fprintln(out, locales.CliNoEditorFor(p.Version))
		return 1
	}
	expected := ""
	for _, e := range res.editors {
		if strings.EqualFold(e.Exe, p.Editor) {
			if e.Simulated {
				fmt.Fprintln(out, locales.SimulatedOpen())
				return 1
			}
			expected = e.Version
			break
		}
	}
	version, _, ok := readProject(p.Path)
	if !ok {
		fmt.Fprintln(out, locales.CliNotAProject(p.Path))
		return 1
	}
	if !strings.EqualFold(version, p.Version) {
		fmt.Fprintln(out, locales.ProjectVersionChanged())
		return 1
	}
	if err := launchVersionCheckedEditor(p.Editor, expected, []string{"-projectPath", p.Path}, filepath.Dir(p.Editor), startDetached); err != nil {
		if errors.Is(err, errEditorVersionChanged) {
			fmt.Fprintln(out, locales.ProjectEditorChanged())
			return 1
		}
		fmt.Fprintln(out, err)
		return 1
	}
	fmt.Fprintln(out, locales.CliOpened(p.Path))
	return 0
}

func cliNew(opts cliOptions, args []string, scanFn func(prefs) scanResult, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, locales.CliUsageHint())
		return 2
	}
	dir := filepath.Clean(args[0])
	settings := loadPrefs()
	res := scanFn(settings)
	editor, ok := pickEditor(res.editors, opts.editor)
	if !ok {
		if opts.editor != "" {
			fmt.Fprintln(out, locales.CliNoEditorFor(opts.editor))
		} else {
			fmt.Fprintln(out, locales.CliNoEditor())
		}
		return 1
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		fmt.Fprintln(out, locales.CliNotEmpty(dir))
		return 1
	}
	if err := startProjectCreation(dir, editor, startDetached); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	fmt.Fprintln(out, locales.CliCreated(dir))
	return 0
}

func cliHub(opts cliOptions, args []string, scanFn func(prefs) scanResult, out io.Writer) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "sync" {
		return cliHubSync(opts, scanFn, out)
	}
	if sub != "status" {
		fmt.Fprintln(out, locales.CliUsageHint())
		return 2
	}
	res := scanFn(loadPrefs())
	if opts.json {
		list := make([]hubJSON, 0, len(res.hubs))
		for _, h := range res.hubs {
			list = append(list, hubJSON{Engine: h.Kind, Version: h.Version, Dir: h.Dir, Mode: hubModeCode(h)})
		}
		return writeJSON(out, list)
	}
	if len(res.hubs) == 0 {
		fmt.Fprintln(out, locales.CliHubNone())
		return 0
	}
	fmt.Fprintln(out, locales.CliHubsHeading())
	cols := table(out)
	for _, h := range res.hubs {
		fmt.Fprintf(cols, "%s\t%s\t%s\t%s\n", hubName(h.Kind), h.Version, hubMode(h), h.Dir)
	}
	cols.Flush()
	return 0
}

// cliHubSync adds the projects added by hand to the hubs that list projects.
func cliHubSync(opts cliOptions, scanFn func(prefs) scanResult, out io.Writer) int {
	return hubSync(loadPrefs(), opts, scanFn, out)
}

func hubSync(settings prefs, opts cliOptions, scanFn func(prefs) scanResult, out io.Writer) int {
	if settings.NoHubSync {
		fmt.Fprintln(out, locales.HubSyncDisabled())
		return 1
	}
	res := scanFn(settings)
	pinned := map[string]bool{}
	for _, dir := range settings.PinnedProjects {
		pinned[filepath.Clean(dir)] = true
	}
	var entries []projectEntry
	for _, p := range res.projects {
		if pinned[p.Path] {
			entries = append(entries, p)
		}
	}
	added, err := exportProjects(res.hubs, entries)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if opts.json {
		return writeJSON(out, map[string]int{"added": added, "pinned": len(entries)})
	}
	fmt.Fprintln(out, locales.CliHubSynced(added, len(entries)))
	return 0
}

func filterProjects(projects []projectEntry, query string) []projectEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return projects
	}
	out := make([]projectEntry, 0, len(projects))
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Path), q) ||
			strings.Contains(strings.ToLower(p.Version), q) {
			out = append(out, p)
		}
	}
	return out
}

// findProject finds a project by its path, else by its name, which must be
// the only one of that name.
func findProject(projects []projectEntry, target string) (projectEntry, string, bool) {
	want := strings.ToLower(filepath.Clean(target))
	for _, p := range projects {
		if strings.ToLower(filepath.Clean(p.Path)) == want {
			return p, "", true
		}
	}
	var matches []projectEntry
	for _, p := range projects {
		if strings.EqualFold(p.Name, target) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], "", true
	case 0:
		return projectEntry{}, locales.CliNotFound(target), false
	default:
		return projectEntry{}, locales.CliAmbiguous(len(matches), target), false
	}
}

// pickEditor is the editor a new project is made with: the version asked
// for, else the newest Unity editor, else the first.
func pickEditor(editors []editorInstall, version string) (editorInstall, bool) {
	if version != "" {
		for _, e := range editors {
			if e.Version == version && !e.Simulated {
				return e, true
			}
		}
		return editorInstall{}, false
	}
	for _, e := range editors {
		if e.Kind == kindUnity && !e.Simulated {
			return e, true
		}
	}
	for _, e := range editors {
		if !e.Simulated {
			return e, true
		}
	}
	return editorInstall{}, false
}

func table(out io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
}

func writeJSON(out io.Writer, v any) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return 0
}

// cliReleases lists the editors the engines offer, with the installed ones
// marked.
func cliReleases(opts cliOptions, scanFn func(prefs) scanResult, out io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	entries, _ := refreshReleaseCatalogs(ctx)
	if len(entries) == 0 {
		fmt.Fprintln(out, locales.DownloadsEmpty())
		return 1
	}
	installed := map[string]bool{}
	for _, e := range scanFn(loadPrefs()).editors {
		installed[e.Kind+" "+e.Version] = true
	}
	if opts.json {
		type releaseJSON struct {
			Version   string `json:"version"`
			Engine    string `json:"engine"`
			LTS       bool   `json:"lts"`
			Stream    string `json:"stream,omitempty"`
			Size      int64  `json:"size"`
			Date      string `json:"date,omitempty"`
			URL       string `json:"url"`
			Installed bool   `json:"installed"`
		}
		list := make([]releaseJSON, 0, len(entries))
		for _, e := range entries {
			date := ""
			if !e.Date.IsZero() {
				date = e.Date.Format("2006-01-02")
			}
			list = append(list, releaseJSON{e.Version, e.Engine, e.LTS, e.Stream, e.Size, date, e.URL, installed[e.Engine+" "+e.Version]})
		}
		return writeJSON(out, list)
	}
	fmt.Fprintln(out, locales.CliReleasesHeading())
	cols := table(out)
	for _, e := range entries {
		mark := ""
		if installed[e.Engine+" "+e.Version] {
			mark = "*"
		}
		fmt.Fprintf(cols, "%s\t%s\t%s\t%s\t%s\n", e.Version, channelName(e.Engine), humanSize(e.Size), mark, e.URL)
	}
	cols.Flush()
	return 0
}

// cliInstall downloads and installs a release, or a stand-in of it.
func cliInstall(opts cliOptions, args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, locales.CliUsageHint())
		return 2
	}
	want := strings.TrimSpace(args[0])
	settings := loadPrefs()
	root := settings.InstallRoot
	if root == "" {
		root = defaultInstallRoot()
	}
	entry, ok := findRelease(want)
	if !ok {
		fmt.Fprintln(out, locales.CliReleaseNotFound(want))
		return 1
	}
	if opts.simulate {
		dir, err := simulateEditorInstall(entry.Engine, entry.Version, root)
		if err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		rememberInstallRoot(&settings, root)
		fmt.Fprintln(out, locales.CliSimulated(entry.Version, dir))
		return 0
	}
	if err := downloadAndInstall(entry, root); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	rememberInstallRoot(&settings, root)
	fmt.Fprintln(out, locales.CliInstalled(entry.Version, filepath.Join(root, entry.Version)))
	return 0
}

func findRelease(version string) (downloadEntry, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if kindOfVersion(version) == kindUnity {
		if entries, err := fetchUnityReleasesForVersion(ctx, version); err == nil {
			for _, e := range entries {
				if e.Version == version {
					return e, true
				}
			}
		}
	}
	entries, _ := refreshReleaseCatalogs(ctx)
	for _, e := range entries {
		if e.Version == version {
			return e, true
		}
	}
	return downloadEntry{}, false
}

// downloadAndInstall runs a release's installer the way the hubs do.
func downloadAndInstall(entry downloadEntry, root string) error {
	if err := validateEditorInstallEntry(entry); err != nil {
		return err
	}
	name := filepath.Base(strings.Split(entry.URL, "?")[0])
	path := filepath.Join(downloadsDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	last := -1
	err := downloadFileVerifiedSize(ctx, entry.URL, path, entry.Integrity, editorDownloadExpectedSize(entry), func(done, total int64) {
		if total <= 0 {
			return
		}
		percent := int(done * 100 / total)
		if percent != last && percent%2 == 0 {
			last = percent
			fmt.Fprintf(os.Stderr, "\r%s %d%%", entry.Version, percent)
		}
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	dest := filepath.Join(root, entry.Version)
	return installVerifiedEditor(entry, path, dest, runEditorInstaller)
}

func rememberInstallRoot(settings *prefs, root string) {
	settings.EditorRoots = dedupe(append(settings.EditorRoots, root))
	if err := savePrefs(*settings); err != nil {
		logger.Printf("settings: %v", err)
	}
}

// cliUpdate says whether a newer release exists.
func cliUpdate(opts cliOptions, out io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	release, err := checkLatestRelease(ctx)
	if err != nil {
		fmt.Fprintln(out, locales.UpdateFailed(err.Error()))
		return 1
	}
	latest := strings.TrimPrefix(release.Tag, "v")
	if opts.json {
		return writeJSON(out, map[string]any{"current": appVersion, "latest": latest, "url": release.URL})
	}
	if newerVersion(release.Tag) {
		fmt.Fprintln(out, locales.UpdateAvailable(latest))
		fmt.Fprintln(out, release.URL)
		return 0
	}
	fmt.Fprintln(out, locales.UpdateLatest())
	return 0
}
