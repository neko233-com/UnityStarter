package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openHarmonyNodeFixture() editorModule {
	return editorModule{ID: openHarmonyNodeModule, Name: "Nodejs", Visible: true, Parent: "openharmony", URL: "https://mirrors.cloud.tencent.com/nodejs-release/v18.14.1/node-v18.14.1-win-x64.zip", Destination: "{UNITY_PATH}/Editor/Data/PlaybackEngines/OpenHarmonyPlayer/Nodejs"}
}

func TestOpenHarmonyNodeMetadataAndQueue(t *testing.T) {
	m := openHarmonyNodeFixture()
	parent := editorModule{ID: "openharmony", Visible: true, URL: "https://download.tuanjie.cn/module.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
	e := editorInstall{Kind: kindTuanjie, Modules: []editorModule{m, parent}}
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("missing parent accepted")
	}
	queue, err := planModuleQueue(e, map[string]bool{m.ID: true, parent.ID: true})
	if err != nil || !reflect.DeepEqual(queue, []string{parent.ID, m.ID}) {
		t.Fatalf("dependency ordering: %v %v", queue, err)
	}
	e.Modules[1].Selected = true
	if _, _, _, ok := moduleInstallSpec(e, m); !ok {
		t.Fatal("valid Node.js module rejected")
	}
	e.Kind = kindUnity
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("Tuanjie module accepted for Unity")
	}
	for _, mutate := range []func(*editorModule){
		func(v *editorModule) { v.URL += "?other=1" },
		func(v *editorModule) { v.Destination += "/.." },
		func(v *editorModule) { v.Parent = "android" },
		func(v *editorModule) { v.EULA = "unknown" },
		func(v *editorModule) { v.RenameTo = "other" },
	} {
		v := m
		mutate(&v)
		if _, ok := openHarmonyNodeArchive(v); ok {
			t.Fatalf("unsafe metadata accepted: %+v", v)
		}
	}
}

func TestOpenHarmonyNodeAtomicInstall(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{openHarmonyNodeFolder + "/node.exe": "node", openHarmonyNodeFolder + "/npm.cmd": "npm", openHarmonyNodeFolder + "/node_modules/npm/bin/npm-cli.js": "cli"}
	for _, unsafe := range []map[string]string{{"../escape": "bad"}, {openHarmonyNodeFolder + "/node.exe": "node"}} {
		archive := makeModuleZip(t, unsafe)
		if err := installOpenHarmonyNode(root, archive, archiveIntegrity(t, archive)); err == nil {
			t.Fatal("incomplete or unsafe archive accepted")
		}
		if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
			t.Fatalf("partial installation remains: %v %v", entries, err)
		}
	}
	archive := makeModuleZip(t, entries)
	if err := installOpenHarmonyNode(root, archive, archiveIntegrity(t, archive)); err != nil {
		t.Fatal(err)
	}
	if !moduleFilesPresent(root, openHarmonyNodeFixture()) {
		t.Fatal("installed Node.js not detected")
	}
	if err := installOpenHarmonyNode(root, archive, archiveIntegrity(t, archive)); err == nil {
		t.Fatal("existing installation overwritten")
	}
	if err := os.Remove(filepath.Join(parent, "Nodejs", openHarmonyNodeFolder, "npm.cmd")); err != nil {
		t.Fatal(err)
	}
	if moduleFilesPresent(root, openHarmonyNodeFixture()) {
		t.Fatal("partial Node.js reported installed")
	}
}

func TestLiveOpenHarmonyNodeArchive(t *testing.T) {
	archive := os.Getenv("UNITYSTARTER_LIVE_OPENHARMONY_NODE")
	if archive == "" {
		t.Skip("set UNITYSTARTER_LIVE_OPENHARMONY_NODE to the official mirrored archive")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	sum, ok := openHarmonyNodeArchive(openHarmonyNodeFixture())
	if !ok {
		t.Fatal("catalog rejected")
	}
	if err := installOpenHarmonyNode(root, archive, sum); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "Nodejs", openHarmonyNodeFolder)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, check := range []struct {
		args []string
		want string
	}{{[]string{"--version"}, "v18.14.1"}, {[]string{filepath.Join(dir, "node_modules", "npm", "bin", "npm-cli.js"), "--version"}, "9.3.1"}} {
		cmd := exec.CommandContext(ctx, filepath.Join(dir, "node.exe"), check.args...)
		hideWindowCmd(cmd)
		if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), check.want) {
			t.Fatalf("tool version: %s %v", out, err)
		}
	}
}

func TestLiveOpenHarmonyNodeDownloadAndRecord(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_NODE_DOWNLOAD") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_NODE_DOWNLOAD=1 to download the catalog package")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	data, err := fetchTuanjieCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m editorModule
	for _, item := range tuanjieModulesFromCatalog(data, "2022.3.62t17") {
		if item.ID == openHarmonyNodeModule {
			m = item
			break
		}
	}
	if _, ok := openHarmonyNodeArchive(m); !ok {
		t.Fatal("current official catalog metadata rejected")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Root: root, Exe: filepath.Join(root, "Editor", "Tuanjie.exe"), Modules: []editorModule{{ID: "openharmony", Selected: true}, m}}
	result := executeEditorModule(ctx, e, m, func(int64, int64) {}, func() error { return nil })
	if result.err != nil || !result.physicalDone {
		t.Fatalf("download/install: %+v", result)
	}
	installed := []editorModule{m}
	applyOwnedModuleState(root, installed)
	if !installed[0].Selected {
		t.Fatal("installed module state was not recorded")
	}
}
