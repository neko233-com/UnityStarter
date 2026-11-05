package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

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
	json   bool
	lang   string
	query  string
	editor string
}

// runCLI runs one command without a window, and returns the exit code.
func runCLI(argv []string, out io.Writer, scanFn func(prefs) scanResult) int {
	opts, args, err := parseCLI(argv)
	if err != nil {
		fmt.Fprintln(out, err)
		return 2
	}
	if opts.lang != "" {
		tag := opts.lang
		i18n.SetLocaleFunc(func() string { return tag })
	}

	command := "help"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
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
	if err := startDetached(p.Editor, []string{"-projectPath", p.Path}, filepath.Dir(p.Editor)); err != nil {
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
	res := scanFn(loadPrefs())
	exe := pickEditor(res.editors, opts.editor)
	if exe == "" {
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if err := startDetached(exe, []string{"-createProject", dir}, filepath.Dir(exe)); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	fmt.Fprintln(out, locales.CliCreated(dir))
	return 0
}

func cliHub(opts cliOptions, args []string, scanFn func(prefs) scanResult, out io.Writer) int {
	if len(args) > 0 && args[0] != "status" {
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
func pickEditor(editors []editorInstall, version string) string {
	if version != "" {
		for _, e := range editors {
			if e.Version == version {
				return e.Exe
			}
		}
		return ""
	}
	for _, e := range editors {
		if e.Kind == kindUnity {
			return e.Exe
		}
	}
	if len(editors) > 0 {
		return editors[0].Exe
	}
	return ""
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
