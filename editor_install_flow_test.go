package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func editorInstallFixture(body []byte) downloadEntry {
	sum := sha256.Sum256(body)
	return downloadEntry{Engine: kindUnity, Version: "7000.0.0a16", URL: "https://download.unity3d.com/download_unity/hash/Windows64EditorInstaller/UnitySetup64-7000.0.0a16.exe", Size: int64(len(body)), Integrity: "sha256-" + base64.StdEncoding.EncodeToString(sum[:])}
}

func TestEditorOneClickWithInstallerFixture(t *testing.T) {
	fixture := os.Getenv("UNITYSTARTER_EDITOR_FIXTURE")
	if fixture == "" {
		t.Skip("set UNITYSTARTER_EDITOR_FIXTURE to an isolated NSIS fixture")
	}
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	e := editorInstallFixture(body)
	root := filepath.Join(t.TempDir(), "Editor location with spaces")
	job := &downloadJob{Entry: e, Root: root, Path: filepath.Join(t.TempDir(), "fixture.exe"), Status: "downloading"}
	a := newApp()
	key := jobKey(e)
	a.jobs = map[string]*downloadJob{key: job}
	err = downloadFileVerifiedSize(context.Background(), server.URL, job.Path, e.Integrity, e.Size, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	var installErr error
	a.finishEditorDownload(key, job, nil, nil, func(job *downloadJob) {
		installErr = installVerifiedEditor(job.Entry, job.Path, filepath.Join(job.Root, job.Entry.Version), runEditorInstaller)
	})
	if installErr != nil {
		t.Fatal(installErr)
	}
	path := filepath.Join(root, e.Version, "Editor", "Unity.exe")
	if got := editorExecutableVersion(path); got != e.Version {
		t.Fatalf("fixture editor version: %q", got)
	}
}

func TestEditorInstallEntryRejectsUnsafeMetadata(t *testing.T) {
	entry := editorInstallFixture([]byte("installer"))
	if err := validateEditorInstallEntry(entry); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*downloadEntry){
		func(e *downloadEntry) { e.URL = "https://example.invalid/UnitySetup64-7000.0.0a16.exe" },
		func(e *downloadEntry) { e.URL = "http://download.unity3d.com/UnitySetup64-7000.0.0a16.exe" },
		func(e *downloadEntry) { e.URL += "?x=1" },
		func(e *downloadEntry) { e.Version = "../escape" },
		func(e *downloadEntry) { e.Version = "" },
		func(e *downloadEntry) { e.Engine = kindTuanjie },
		func(e *downloadEntry) { e.Integrity = "" },
	} {
		bad := entry
		mutate(&bad)
		if err := validateEditorInstallEntry(bad); err == nil {
			t.Fatalf("unsafe release accepted: %+v", bad)
		}
	}
}

func TestEditorDestinationPreflightPreservesExistingInstallation(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "7000.0.0a16")
	if err := checkEditorInstallDestination(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkEditorInstallDestination(dest); err == nil {
		t.Fatal("existing directory accepted before downloading")
	}
	if err := os.WriteFile(filepath.Join(dest, installMarker), []byte("interrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkEditorInstallDestination(dest); err != nil {
		t.Fatalf("interrupted installation cannot resume: %v", err)
	}
}

func TestEditorDownloadTransitionsAndCancellation(t *testing.T) {
	e := editorInstallFixture([]byte("installer"))
	key := jobKey(e)
	a := newApp()
	job := &downloadJob{Entry: e, Root: "original location", Status: "downloading", Total: 20}
	a.jobs = map[string]*downloadJob{key: job}
	called := 0
	install := func(got *downloadJob) {
		called++
		if got != job || got.Root != "original location" {
			t.Fatal("changed job or location")
		}
	}
	a.finishEditorDownload(key, job, nil, nil, install)
	if called != 1 || job.Status != "downloaded" || job.Done != 20 {
		t.Fatal("download did not transition to installation")
	}
	job.Status = "downloading"
	a.finishEditorDownload(key, job, nil, context.Canceled, install)
	if called != 1 || a.jobs[key] != nil {
		t.Fatal("canceled download started installation")
	}
	a.jobs[key] = job
	a.finishEditorDownload(key, job, errors.New("offline"), nil, install)
	if called != 1 || job.Status != "failedDownload" {
		t.Fatal("failed download started installation")
	}
	a.jobs[key] = &downloadJob{}
	a.finishEditorDownload(key, job, nil, nil, install)
	if called != 1 {
		t.Fatal("stale download started installation")
	}
}

func TestEditorDownloadRetryKeepsOriginalLocation(t *testing.T) {
	for _, status := range []string{"failedDownload", "failedInstall"} {
		if root, ok := editorDownloadRoot(&downloadJob{Status: status, Root: "original"}, "new default"); !ok || root != "original" {
			t.Fatalf("%s retry changed install location: %q %v", status, root, ok)
		}
	}
	for _, status := range []string{"downloading", "downloaded", "installing", "installed", "clearing"} {
		if _, ok := editorDownloadRoot(&downloadJob{Status: status}, "new default"); ok {
			t.Fatalf("%s job was replaced", status)
		}
	}
	if root, ok := editorDownloadRoot(nil, "new default"); !ok || root != "new default" {
		t.Fatalf("new job did not use default: %q %v", root, ok)
	}
}

func TestFailedEditorInstallOffersDownloadRecovery(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	e := editorInstallFixture([]byte("fixture"))
	a.downloads = []downloadEntry{e}
	a.jobs = map[string]*downloadJob{jobKey(e): {Entry: e, Status: "failedInstall", Err: "invalid installer"}}
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.ActionInstall()) || !tt.HasText(locales.ActionRetryDownloadInstall()) {
		t.Fatalf("install recovery actions missing: %q", tt.Texts())
	}
	if path := os.Getenv("INSTALL_RECOVERY_SNAPSHOT"); path != "" {
		tt.Frame()
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUninstalledEditorCanBeInstalledAgain(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	e := editorInstallFixture([]byte("fixture"))
	a.downloads = []downloadEntry{e}
	key := jobKey(e)
	a.jobs = map[string]*downloadJob{key: {Entry: e, Status: "installed"}}
	a.editors = []editorInstall{{Kind: e.Engine, Version: e.Version}}
	a.clearConfirmedDownloadJobs()
	if _, exists := a.jobs[key]; exists {
		t.Fatal("completed job was not cleared after the installation was found")
	}
	a.editors = nil // A later uninstall is reflected by the next scan.
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.DownloadAndInstall()) {
		t.Fatalf("reinstall action missing after uninstall: %q", tt.Texts())
	}
	a.jobs[key] = &downloadJob{Entry: e, Status: "installed"}
	a.clearConfirmedDownloadJobs()
	if a.jobs[key] == nil {
		t.Fatal("unconfirmed completed job was cleared before the scan found it")
	}
}

func TestEditorInstallReverifiesBeforeLaunching(t *testing.T) {
	body := []byte("verified installer")
	e := editorInstallFixture(body)
	root := t.TempDir()
	installer := filepath.Join(root, "installer.exe")
	if err := os.WriteFile(installer, body, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, e.Version)
	called := 0
	run := func(_, engine, dir string) error {
		called++
		if engine != kindUnity {
			t.Fatal("wrong engine")
		}
		if err := os.MkdirAll(filepath.Join(dir, "Editor"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "Editor", "Unity.exe"), []byte("fixture"), 0o600)
	}
	verify := func(engine, version, dir string) error { return nil }
	if err := os.WriteFile(installer, []byte("tampered installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installVerifiedEditorWith(e, installer, dest, run, verify); err == nil || called != 0 {
		t.Fatal("tampered installer executed")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("failed verification created an install directory")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	if err := downloadFileVerifiedSize(context.Background(), server.URL, installer, e.Integrity, e.Size, func(int64, int64) {}); err != nil {
		t.Fatalf("corrupt installer recovery failed: %v", err)
	}
	if err := installVerifiedEditorWith(e, installer, dest, run, verify); err != nil || called != 1 {
		t.Fatalf("verified installation failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, installMarker)); !os.IsNotExist(err) {
		t.Fatal("successful install marker remains")
	}
	if err := installVerifiedEditorWith(e, installer, dest, run, verify); err == nil || called != 1 {
		t.Fatal("existing editor overwritten")
	}
}

func TestEditorInstallRejectsIncompleteOrWrongVersion(t *testing.T) {
	body := []byte("verified installer")
	e := editorInstallFixture(body)
	root := t.TempDir()
	installer := filepath.Join(root, "installer.exe")
	if err := os.WriteFile(installer, body, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, e.Version)
	run := func(_, _, dir string) error {
		if err := os.MkdirAll(filepath.Join(dir, "Editor"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "Editor", "Unity.exe"), []byte("fixture"), 0o600)
	}
	version := e.Version
	verify := func(engine, expected, dir string) error {
		return verifyInstalledEditorWithVersion(engine, expected, dir, func(string) string { return version })
	}
	if err := installVerifiedEditorWith(e, installer, dest, run, verify); err == nil {
		t.Fatal("missing core files accepted")
	}
	if _, err := os.Stat(filepath.Join(dest, installMarker)); err != nil {
		t.Fatal("incomplete installation marker missing")
	}
	if got := scanEditorsIn([]string{dest}); len(got) != 0 {
		t.Fatalf("incomplete editor was visible: %+v", got)
	}
	for _, dir := range []string{"Editor/Data/Managed", "Editor/Data/Resources", "Editor/Data/PlaybackEngines"} {
		if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"UnityEngine.dll", "UnityEditor.dll"} {
		if err := os.WriteFile(filepath.Join(dest, "Editor", "Data", "Managed", name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	version = "6000.0.0f1"
	if err := installVerifiedEditorWith(e, installer, dest, run, verify); err == nil {
		t.Fatal("wrong version accepted")
	}
	if _, err := os.Stat(filepath.Join(dest, installMarker)); err != nil {
		t.Fatal("wrong-version marker missing")
	}
	version = e.Version
	if err := installVerifiedEditorWith(e, installer, dest, run, verify); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, installMarker)); !os.IsNotExist(err) {
		t.Fatal("validated installation retained marker")
	}
}

func TestInstalledRealEditorVersionAndCoreFiles(t *testing.T) {
	root := os.Getenv("UNITYSTARTER_REAL_EDITOR_ROOT")
	version := os.Getenv("UNITYSTARTER_REAL_EDITOR_VERSION")
	engine := os.Getenv("UNITYSTARTER_REAL_EDITOR_ENGINE")
	if root == "" || version == "" || engine == "" {
		t.Skip("explicit installed editor validation")
	}
	if err := verifyInstalledEditor(engine, version, root); err != nil {
		t.Fatal(err)
	}
	if err := verifyInstalledEditor(engine, "0.0.0f1", root); err == nil {
		t.Fatal("wrong version accepted")
	}
	t.Logf("verified editor product version and core files: %s %s", engine, version)
}

func TestDownloadAndInstallActionShown(t *testing.T) {
	a := fixture()
	a.page = pageDownloads
	a.downloads = []downloadEntry{editorInstallFixture([]byte("fixture"))}
	a.jobs = nil
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.DownloadAndInstall()) {
		t.Fatalf("install action missing: %q", tt.Texts())
	}
	a.jobs = map[string]*downloadJob{"running": {Status: "installing"}}
	if !a.installingEditor() {
		t.Fatal("running installation not detected")
	}
}
