package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

// The queue drives the production download/install function using official
// language packages and an isolated editor tree. No real editor is changed.
func TestOfficialLanguageBatchInstallation(t *testing.T) {
	if os.Getenv("UNITYSTARTER_TEST_OFFICIAL_BATCH") != "1" {
		t.Skip("set UNITYSTARTER_TEST_OFFICIAL_BATCH=1 for official downloads")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "Editor", "Tuanjie.exe")
	if err := os.WriteFile(exe, []byte("isolated fixture; not executed"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := languageFixture()
	first.Name, first.Visible = "Chinese", true
	second := languageFixture()
	second.ID, second.Name, second.Visible = "language-ja", "Japanese", true
	second.URL = strings.TrimSuffix(second.URL, "zh-hans") + "ja"
	e := editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Root: root, Exe: exe, Modules: []editorModule{first, second}}
	metadata, err := json.Marshal(e.Modules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.manageEditor, a.manageOpen, a.manageModules = e, true, true
	var attempted []string
	a.moduleQueueRunner = func(current editorInstall, m editorModule, android bool) {
		if android {
			t.Fatal("unexpected Android consent")
		}
		attempted = append(attempted, m.ID)
		result := executeEditorModule(context.Background(), current, m, func(int64, int64) {}, func() error { return nil })
		a.finishEditorModuleInstall(current, m, result)
	}
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click("Chinese"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Japanese"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.ModuleBatchInstall(2)); err != nil {
		t.Fatal(err)
	}
	if len(attempted) != 2 || len(a.moduleQueue) != 0 || len(a.moduleSelection) != 0 || a.moduleStatus != locales.ModuleBatchDone() {
		t.Fatalf("batch incomplete: attempted=%v remaining=%v status=%q", attempted, a.moduleSelection, a.moduleStatus)
	}
	for _, m := range []editorModule{first, second} {
		language, _, _ := editorLanguageModule(m)
		data, err := os.ReadFile(filepath.Join(root, "Editor", "Data", "Localization", language+".po"))
		if err != nil || !validLanguagePO(data, language) {
			t.Fatalf("%s file invalid: %v", language, err)
		}
	}
	for _, m := range readEditorModules(root) {
		if !m.Selected || m.PresenceUnverified {
			t.Fatalf("module status not verified: %s", m.ID)
		}
	}
	t.Log("two official language packages downloaded, installed and recorded in an isolated editor tree")
}

func TestOfficialLanguageBatchPreservesExistingFileOnFailure(t *testing.T) {
	if os.Getenv("UNITYSTARTER_TEST_OFFICIAL_BATCH") != "1" {
		t.Skip("set UNITYSTARTER_TEST_OFFICIAL_BATCH=1 for official downloads")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	localization := filepath.Join(root, "Editor", "Data", "Localization")
	if err := os.MkdirAll(localization, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "Editor", "Tuanjie.exe")
	if err := os.WriteFile(exe, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := languageFixture()
	first.Name, first.Visible = "Chinese", true
	second := languageFixture()
	second.ID, second.Name, second.Visible = "language-ja", "Japanese", true
	second.URL = strings.TrimSuffix(second.URL, "zh-hans") + "ja"
	existing := []byte("existing user file")
	if err := os.WriteFile(filepath.Join(localization, "ja.po"), existing, 0o600); err != nil {
		t.Fatal(err)
	}
	e := editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Root: root, Exe: exe, Modules: []editorModule{first, second}}
	metadata, err := json.Marshal(e.Modules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.manageEditor, a.manageOpen, a.manageModules = e, true, true
	a.moduleSelection = map[string]bool{first.ID: true, second.ID: true}
	queue, err := planModuleQueue(e, a.moduleSelection)
	if err != nil {
		t.Fatal(err)
	}
	a.moduleQueue, a.moduleQueueTotal = queue, len(queue)
	a.moduleQueueRunner = func(current editorInstall, m editorModule, android bool) {
		result := executeEditorModule(context.Background(), current, m, func(int64, int64) {}, func() error { return nil })
		a.finishEditorModuleInstall(current, m, result)
	}
	a.advanceModuleQueue()
	got, err := os.ReadFile(filepath.Join(localization, "ja.po"))
	if err != nil || string(got) != string(existing) {
		t.Fatal("existing file was changed")
	}
	if len(a.moduleQueue) != 0 || a.moduleSelection[first.ID] || !a.moduleSelection[second.ID] || a.moduleStatus == locales.ModuleBatchDone() {
		t.Fatalf("failure state: queue=%v selected=%v status=%q", a.moduleQueue, a.moduleSelection, a.moduleStatus)
	}
	modules := readEditorModules(root)
	if len(modules) != 2 || !modules[0].Selected || modules[1].Selected {
		t.Fatalf("incorrect installed records: %+v", modules)
	}
	t.Log("completed module retained; conflicting user file preserved; failed module remains selected")
}

func TestOfficialLanguageCancellationBeforeInstall(t *testing.T) {
	if os.Getenv("UNITYSTARTER_TEST_OFFICIAL_BATCH") != "1" {
		t.Skip("set UNITYSTARTER_TEST_OFFICIAL_BATCH=1 for official downloads")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "Editor", "Tuanjie.exe")
	if err := os.WriteFile(exe, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := languageFixture()
	m.Visible = true
	e := editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17", Root: root, Exe: exe, Modules: []editorModule{m}}
	result := executeEditorModule(context.Background(), e, m, func(int64, int64) {}, func() error { return context.Canceled })
	if result.err != context.Canceled || result.physicalDone {
		t.Fatalf("cancellation result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "Editor", "Data", "Localization", "zh-hans.po")); !os.IsNotExist(err) {
		t.Fatalf("canceled module created a file: %v", err)
	}
	if len(readModuleState(root)) != 0 {
		t.Fatal("canceled module marked installed")
	}
	t.Log("cancellation after download leaves no installed file or state")
}
