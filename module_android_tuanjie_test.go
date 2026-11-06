package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func legacyTuanjieFixture() (editorInstall, editorModule) {
	modules := []editorModule{{ID: "android", Selected: true}}
	for i, spec := range legacyTuanjieAndroidArchives {
		m := editorModule{ID: spec.id, URL: googleArchiveBase + spec.filename, Destination: androidPlayerPlaceholder + spec.destination}
		if spec.renameFrom != "" {
			m.RenameFrom = androidPlayerPlaceholder + spec.renameFrom
		}
		if spec.renameTo != "" {
			m.RenameTo = androidPlayerPlaceholder + spec.renameTo
		}
		if i == 0 {
			m.Parent, m.Sync, m.EULA = "android", "Android Build Support", androidLicenseURL
		} else {
			m.Sync = "android-sdk-ndk-tools"
		}
		modules = append(modules, m)
	}
	annotateSDKDependencies(modules)
	return editorInstall{Kind: kindTuanjie, Modules: modules}, modules[1]
}

func TestLegacyTuanjieLicenseExplainsChecksumBaseline(t *testing.T) {
	a := fixture()
	a.manageOpen, a.manageModules = true, true
	a.manageEditor, _ = legacyTuanjieFixture()
	a.androidPlan = &androidInstallPlan{License: "fixture terms", LegacyTuanjie: true}
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.AndroidLegacyBaseline()) || !tt.HasText(locales.AndroidLicenseAccept()) {
		t.Fatalf("legacy provenance or license action missing: %q", tt.Texts())
	}
}

func TestLegacyTuanjieRecoveryAfterInterruptedCommit(t *testing.T) {
	root, base := fakeEditorRoot(t), t.TempDir()
	contents := []map[string]string{
		{"tools/bin/sdkmanager.bat": "old sdk tools"},
		{"platform-tools/adb.exe": "adb"},
		{"android-14/aapt.exe": "aapt"},
		{"android-13/android.jar": "33"},
		{"android-34/android.jar": "34"},
		{"android-35/android.jar": "35"},
		{"android-ndk-r23b/source.properties": "ndk"},
		{"cmdline-tools/bin/sdkmanager.bat": "cmdline"},
	}
	plan := androidInstallPlan{Module: editorModule{ID: "android-sdk-ndk-tools"}, License: "fixture terms", LegacyTuanjie: true}
	var archives []string
	for i, spec := range legacyTuanjieAndroidArchives {
		archive := makeModuleZip(t, contents[i])
		info, err := os.Stat(archive)
		if err != nil {
			t.Fatal(err)
		}
		archives = append(archives, archive)
		plan.Packages = append(plan.Packages, androidPackage{Module: editorModule{ID: spec.id}, Integrity: archiveIntegrity(t, archive), ExpectedSize: info.Size(), Destination: spec.destination, RenameFrom: spec.renameFrom, RenameTo: spec.renameTo})
	}
	fixture := androidCrashFixture{Root: root, Base: base, Plan: plan, Archives: archives}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(spec, data, 0o600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAndroidCommitCrashChild$")
	cmd.Env = append(os.Environ(), "UNITYSTARTER_ANDROID_COMMIT_CHILD="+spec, "UNITYSTARTER_ANDROID_COMMIT_PHASE=SDK")
	hideWindowCmd(cmd)
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 91 {
		t.Fatalf("child did not stop after SDK commit: %s %v", out, err)
	}
	cache, err := openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	recovered, err := recoverAndroidJournal(cache, root)
	player := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
	if err != nil || !recovered || !androidSDKFilesPresentForPlan(player, true) {
		t.Fatalf("legacy recovery failed: recovered=%v error=%v", recovered, err)
	}
	if err := finalizeAndroidJournal(cache, root); err != nil {
		t.Fatal(err)
	}
}

func TestLiveTuanjieAndroidPlan(t *testing.T) {
	root := os.Getenv("UNITYSTARTER_TUANJIE_EDITOR_ROOT")
	if root == "" {
		t.Skip("explicit live metadata check")
	}
	modules := readEditorModules(root)
	var sdk editorModule
	for i := range modules {
		if modules[i].ID == "android" {
			modules[i].Selected = true
		}
		if modules[i].ID == "android-sdk-ndk-tools" {
			sdk = modules[i]
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	plan, err := prepareAndroidPlan(ctx, editorInstall{Kind: kindTuanjie, Modules: modules}, sdk)
	if err != nil || !plan.LegacyTuanjie || len(plan.Packages) != 8 || len(plan.License) < 1000 {
		t.Fatalf("real plan: count=%d license=%d error=%v", len(plan.Packages), len(plan.License), err)
	}
	t.Logf("real Tuanjie metadata and official license: %d archives", len(plan.Packages))
}

func TestLegacyTuanjieAndroidPlanValidatesEveryDependency(t *testing.T) {
	e, m := legacyTuanjieFixture()
	plan, err := prepareLegacyTuanjieAndroidPlan(e, m, "fixture terms")
	if err != nil || !plan.LegacyTuanjie || len(plan.Packages) != 8 || !androidSDKCandidate(e, m) {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	for _, mutate := range []func(*editorInstall){
		func(e *editorInstall) { e.Modules[2].URL = "https://example.invalid/file.zip" },
		func(e *editorInstall) { e.Modules[3].RenameTo += "/outside" },
		func(e *editorInstall) { e.Modules = e.Modules[:len(e.Modules)-1] },
		func(e *editorInstall) { e.Modules = append(e.Modules, e.Modules[2]) },
	} {
		bad := e
		bad.Modules = append([]editorModule(nil), e.Modules...)
		mutate(&bad)
		if _, err := prepareLegacyTuanjieAndroidPlan(bad, m, "fixture terms"); err == nil {
			t.Fatal("unsafe or incomplete metadata accepted")
		}
	}
}

func TestLegacyTuanjieRealArchivesInIsolatedEditor(t *testing.T) {
	dir := os.Getenv("UNITYSTARTER_LEGACY_ARCHIVES")
	metadataRoot := os.Getenv("UNITYSTARTER_TUANJIE_EDITOR_ROOT")
	if dir == "" || metadataRoot == "" {
		t.Skip("explicit real-archive integration test")
	}
	modules := readEditorModules(metadataRoot)
	var rootModule editorModule
	for i := range modules {
		if modules[i].ID == "android" {
			modules[i].Selected = true
		}
		if modules[i].ID == "android-sdk-ndk-tools" {
			rootModule = modules[i]
		}
	}
	e := editorInstall{Kind: kindTuanjie, Modules: modules}
	plan, err := prepareLegacyTuanjieAndroidPlan(e, rootModule, "isolated test terms, not user consent")
	if err != nil {
		t.Fatal(err)
	}
	var archives []string
	for _, spec := range legacyTuanjieAndroidArchives {
		archives = append(archives, filepath.Join(dir, spec.filename))
	}
	root := fakeEditorRoot(t)
	data, err := json.Marshal(modules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := openAndroidCache(t.TempDir(), root, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	if err := installAndroidArchivesJournal(root, plan, archives, cache, nil); err != nil {
		t.Fatal(err)
	}
	if err := markModuleInstalled(root, rootModule); err != nil {
		t.Fatal(err)
	}
	if err := finalizeAndroidJournal(cache, root); err != nil {
		t.Fatal(err)
	}
	got := readEditorModules(root)
	for _, m := range got {
		if (m.ID == rootModule.ID || m.Sync == rootModule.ID) && !m.Selected {
			t.Fatalf("module not recorded: %s", m.ID)
		}
	}
	t.Log("all eight real archives verified, extracted, committed and recorded in an isolated editor")
}
