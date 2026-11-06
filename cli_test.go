package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unitystarter/locales"
)

func fakeScan() scanResult {
	return scanResult{
		editors: []editorInstall{
			{Version: "2022.3.51f1", Kind: kindUnity, Exe: `C:\Unity\2022.3.51f1\Editor\Unity.exe`},
			{Version: "2022.3.62t17", Kind: kindTuanjie, Exe: `D:\IDE\TuanJieEditor\2022.3.62t17\Editor\Tuanjie.exe`},
		},
		projects: []projectEntry{
			{Name: "demo", Path: `D:\Games\demo`, Version: "2022.3.51f1", Kind: kindUnity, Editor: `C:\Unity\2022.3.51f1\Editor\Unity.exe`, Favorite: true, Hub: kindUnity},
			{Name: "demo", Path: `D:\Games\demo-copy`, Version: "2022.3.51f1", Kind: kindUnity},
			{Name: "cozy", Path: `D:\Games\cozy`, Version: "2022.3.62t17", Kind: kindTuanjie},
		},
		hubs: []hub{{Kind: kindUnity, Dir: `C:\Hub`, Version: "3.3.7", Registry: `C:\Hub\projects-v1.json`}},
	}
}

func run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := runCLI(args, &buf, func(prefs) scanResult { return fakeScan() })
	return code, buf.String()
}

func TestCLIProjectsJSON(t *testing.T) {
	code, out := run(t, "projects", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	var list []projectJSON
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(list) != 3 {
		t.Fatalf("projects %v", list)
	}
	if !list[0].Favorite || list[0].Hub != kindUnity {
		t.Errorf("first project %+v", list[0])
	}
}

func TestCLIProjectsQuery(t *testing.T) {
	code, out := run(t, "projects", "--query", "cozy")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(out, "cozy") || strings.Contains(out, "demo") {
		t.Errorf("output %q", out)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	code, out := run(t, "nonsense")
	if code != 2 {
		t.Errorf("exit %d", code)
	}
	if !strings.Contains(out, "nonsense") {
		t.Errorf("output %q", out)
	}
}

func TestCLIHelp(t *testing.T) {
	code, out := run(t, "help")
	if code != 0 || !strings.Contains(out, "unitystarter projects") {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestCLIOpenAmbiguous(t *testing.T) {
	code, out := run(t, "open", "demo")
	if code != 1 {
		t.Errorf("exit %d", code)
	}
	if !strings.Contains(out, "demo") {
		t.Errorf("output %q", out)
	}
}

func TestCLIOpenMissing(t *testing.T) {
	if code, _ := run(t, "open", "nowhere"); code != 1 {
		t.Errorf("exit %d", code)
	}
}

func TestCLIOpenRejectsReplacedOrMissingEditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game")
	writeProject(t, path, "2022.3.51f1")
	project := projectEntry{Name: "game", Path: path, Version: "2022.3.51f1", Kind: kindUnity, Editor: "editor"}
	for _, editors := range [][]editorInstall{
		nil,
		{{Exe: "editor", Version: "6000.0.1f1", Kind: kindUnity}},
		{{Exe: "editor", Version: "2022.3.51f1", Kind: kindTuanjie}},
	} {
		res := scanResult{editors: editors, projects: []projectEntry{project}}
		var out bytes.Buffer
		if code := runCLI([]string{"open", path}, &out, func(prefs) scanResult { return res }); code != 1 || !strings.Contains(out.String(), locales.ProjectEditorChanged()) {
			t.Fatalf("stale editor accepted: exit=%d output=%q", code, out.String())
		}
	}
}

func TestCLINeverLaunchesSimulatedEditors(t *testing.T) {
	simulated := editorInstall{Version: "6000.0.2f1", Kind: kindUnity, Exe: "simulated", Simulated: true}
	real := editorInstall{Version: simulated.Version, Kind: kindUnity, Exe: "real"}
	if got, ok := pickEditor([]editorInstall{simulated, real}, simulated.Version); !ok || got.Exe != real.Exe {
		t.Fatalf("new project picked a simulated editor: %+v %v", got, ok)
	}
	if _, ok := pickEditor([]editorInstall{simulated}, ""); ok {
		t.Fatal("new project accepted only a simulated editor")
	}
	var out bytes.Buffer
	res := scanResult{editors: []editorInstall{simulated}, projects: []projectEntry{{Name: "game", Path: "game", Editor: simulated.Exe}}}
	if code := runCLI([]string{"open", "game"}, &out, func(prefs) scanResult { return res }); code != 1 || !strings.Contains(out.String(), locales.SimulatedOpen()) {
		t.Fatalf("simulated editor should be rejected: exit=%d output=%q", code, out.String())
	}
}

func TestCLIOpenRejectsStaleProjectVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game")
	writeProject(t, path, "6000.0.2f1")
	res := scanResult{projects: []projectEntry{{Name: "game", Path: path, Version: "2022.3.51f1", Editor: "editor"}}}
	var out bytes.Buffer
	code := runCLI([]string{"open", path}, &out, func(prefs) scanResult { return res })
	if code != 1 || !strings.Contains(out.String(), locales.ProjectVersionChanged()) {
		t.Fatalf("stale project launch was not rejected: exit=%d output=%q", code, out.String())
	}
}

func TestCLIOpenRejectsChangedRealEditor(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_LIVE_UNITY_EDITOR")
	if exe == "" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_EDITOR to an installed editor")
	}
	if actual := editorExecutableVersion(exe); actual == "" || actual == "6000.0.1f1" {
		t.Fatalf("unexpected editor version %q", actual)
	}
	path := filepath.Join(t.TempDir(), "Game")
	writeProject(t, path, "6000.0.1f1")
	res := scanResult{editors: []editorInstall{{Version: "6000.0.1f1", Kind: kindUnity, Exe: exe}}, projects: []projectEntry{{Name: "game", Path: path, Version: "6000.0.1f1", Kind: kindUnity, Editor: exe}}}
	var out bytes.Buffer
	if code := runCLI([]string{"open", path}, &out, func(prefs) scanResult { return res }); code != 1 || !strings.Contains(out.String(), locales.ProjectEditorChanged()) {
		t.Fatalf("changed editor not rejected: exit=%d output=%q", code, out.String())
	}
}

func TestCLIHubStatusJSON(t *testing.T) {
	code, out := run(t, "hub", "status", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	var hubs []hubJSON
	if err := json.Unmarshal([]byte(out), &hubs); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(hubs) != 1 || hubs[0].Mode != "two-way" {
		t.Errorf("hubs %+v", hubs)
	}
}

func TestCLINewRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := run(t, "new", dir); code != 1 {
		t.Errorf("exit %d", code)
	}
}

func TestCLIVersion(t *testing.T) {
	code, out := run(t, "version")
	if code != 0 || !strings.Contains(out, appVersion) {
		t.Errorf("exit %d, output %q", code, out)
	}
}
