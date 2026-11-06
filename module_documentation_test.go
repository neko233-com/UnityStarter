package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func documentationFixtureModule() editorModule {
	return editorModule{ID: "documentation", URL: "https://download.tuanjie.cn/download_docs/2022.3.62t17/TuanjieDocumentation.zip", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}/Editor/Data", DownloadSize: 123}
}

func unityDocumentationFixtureModule() editorModule {
	return editorModule{ID: "documentation", URL: "https://cloudmedia-docs.unity3d.com/docscloudstorage/2022.3/UnityDocumentation.zip", Destination: "{UNITY_PATH}/Editor/Data", DownloadSize: 346341182}
}

func TestUnityDocumentationMetadataAndResponse(t *testing.T) {
	m := unityDocumentationFixtureModule()
	e := editorInstall{Kind: kindUnity, Version: "2022.3.51f1"}
	if _, player, _, ok := moduleInstallSpec(e, m); !ok || player != unityDocumentationModule {
		t.Fatal("official Unity documentation rejected")
	}
	for _, mutate := range []func(*editorModule){
		func(m *editorModule) {
			m.URL = strings.Replace(m.URL, "cloudmedia-docs.unity3d.com", "attacker.invalid", 1)
		},
		func(m *editorModule) { m.URL += "?extra=1" },
		func(m *editorModule) { m.Destination += "/other" },
		func(m *editorModule) { m.EULA = "https://example.com" },
		func(m *editorModule) { m.DownloadSize = 0 },
	} {
		bad := m
		mutate(&bad)
		if _, ok := unityDocumentationArchive(bad); ok {
			t.Fatalf("unsafe metadata accepted: %+v", bad)
		}
	}
	e.Version = "6000.0.1f1"
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("wrong Unity documentation version accepted")
	}
	e.Kind, e.Version = kindTuanjie, "2022.3.62t17"
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("Unity documentation enabled for Tuanjie")
	}
	m.URL = strings.Replace(m.URL, "/2022.3/", "/7000.0/", 1)
	e.Kind, e.Version = kindUnity, "7000.0.0a16"
	if _, player, _, ok := moduleInstallSpec(e, m); !ok || player != unityDocumentationModule {
		t.Fatal("Unity 7 documentation metadata rejected")
	}
	response := &http.Response{StatusCode: 200, ContentLength: 346563056, Header: http.Header{"X-Goog-Hash": []string{"crc32c=abcd, md5=WieLUWvquoKPvG9P59/HwQ=="}}}
	if sum, size, err := unityDocumentationResponse(response); err != nil || sum != "md5-WieLUWvquoKPvG9P59/HwQ==" || size != 346563056 {
		t.Fatalf("response rejected: %q %d %v", sum, size, err)
	}
	response.Header.Set("X-Goog-Hash", "md5=invalid")
	if _, _, err := unityDocumentationResponse(response); err == nil {
		t.Fatal("invalid server digest accepted")
	}
}

func TestUnityDocumentationAtomicInstall(t *testing.T) {
	root := documentationFixtureRoot(t)
	m := unityDocumentationFixtureModule()
	archive := makeModuleZip(t, map[string]string{"Documentation/en/Manual/index.html": "manual", "Documentation/en/ScriptReference/index.html": "api"})
	if err := installDocumentation(root, archive, documentationFixtureIntegrity(t, archive), "en"); err != nil {
		t.Fatal(err)
	}
	if !moduleFilesPresent(root, m) {
		t.Fatal("Unity documentation not detected")
	}
	if err := installDocumentation(root, archive, documentationFixtureIntegrity(t, archive), "en"); err == nil {
		t.Fatal("existing documentation overwritten")
	}
}

func TestLiveUnityDocumentationArchive(t *testing.T) {
	archive := os.Getenv("UNITYSTARTER_LIVE_UNITY_DOCS_ARCHIVE")
	if archive == "" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_DOCS_ARCHIVE to the official archive")
	}
	m := unityDocumentationFixtureModule()
	sum, _, err := unityDocumentationRemote(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	root := documentationFixtureRoot(t)
	if err := installDocumentation(root, archive, sum, "en"); err != nil {
		t.Fatal(err)
	}
	if !moduleFilesPresent(root, m) {
		t.Fatal("official Unity documentation not detected")
	}
}

func TestLiveUnityDocumentationDownloadAndInstall(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_UNITY_DOCS_NETWORK") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_DOCS_NETWORK=1 to download the official archive")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := unityDocumentationFixtureModule()
	m.Name, m.Visible = "Documentation", true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sum, size, err := unityDocumentationRemote(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	root := documentationFixtureRoot(t)
	archive := filepath.Join(t.TempDir(), "UnityDocumentation.zip")
	if err := downloadFileVerifiedSize(ctx, m.URL, archive, sum, size, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if err := installDocumentation(root, archive, sum, "en"); err != nil {
		t.Fatal(err)
	}
	if err := markModuleInstalled(root, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Editor", "Unity.exe"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	modules := []editorModule{m}
	applyOwnedModuleState(root, modules)
	editors := scanEditorsIn([]string{root})
	if !modules[0].Selected || len(editors) != 1 || editors[0].OfflineDocs == "" {
		t.Fatalf("Unity documentation not recorded and discovered: %+v %+v", modules, editors)
	}
}

func documentationFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func documentationFixtureIntegrity(t *testing.T, archive string) string {
	t.Helper()
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(data)
	return "md5-" + base64.StdEncoding.EncodeToString(sum[:])
}

func TestTuanjieDocumentationMetadataValidation(t *testing.T) {
	m := documentationFixtureModule()
	e := editorInstall{Kind: kindTuanjie, Version: "2022.3.62t17"}
	if _, player, ext, ok := moduleInstallSpec(e, m); !ok || player != documentationModule || ext != ".zip" {
		t.Fatalf("official metadata rejected: %+v", m)
	}
	for _, change := range []func(*editorModule){
		func(m *editorModule) { m.URL = strings.Replace(m.URL, "download.tuanjie.cn", "example.com", 1) },
		func(m *editorModule) { m.URL += "?unexpected=1" },
		func(m *editorModule) { m.Destination = "{UNITY_PATH}/Editor" },
		func(m *editorModule) { m.EULA = "https://example.com/license" },
		func(m *editorModule) { m.Checksum = "bad" },
		func(m *editorModule) { m.DownloadSize = 0 },
	} {
		bad := m
		change(&bad)
		if _, _, _, ok := moduleInstallSpec(e, bad); ok {
			t.Fatalf("invalid metadata accepted: %+v", bad)
		}
	}
	e.Version = "2022.3.62t15"
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("another editor version accepted")
	}
	e.Kind, e.Version = kindUnity, "2022.3.62t17"
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("wrong engine accepted")
	}
}

func TestTuanjieDocumentationAtomicInstallAndExistingFiles(t *testing.T) {
	root := documentationFixtureRoot(t)
	archive := makeModuleZip(t, map[string]string{
		"Documentation/zh_CN/Manual/index.html":          "manual",
		"Documentation/zh_CN/ScriptReference/index.html": "api",
	})
	if err := installTuanjieDocumentation(root, archive, documentationFixtureIntegrity(t, archive)); err != nil {
		t.Fatal(err)
	}
	if !moduleFilesPresent(root, documentationFixtureModule()) {
		t.Fatal("installed documentation was not detected")
	}
	if err := os.WriteFile(filepath.Join(root, "Editor", "Tuanjie.exe"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	editors := scanEditorsIn([]string{root})
	if len(editors) != 1 || editors[0].OfflineDocs != filepath.Join(root, "Editor", "Data", "Documentation") {
		t.Fatalf("offline documentation not discovered: %+v", editors)
	}
	if err := installTuanjieDocumentation(root, archive, documentationFixtureIntegrity(t, archive)); err == nil {
		t.Fatal("existing documentation was overwritten")
	}
	path := filepath.Join(root, "Editor", "Data", "Documentation", "zh_CN", "Manual", "index.html")
	if got, err := os.ReadFile(path); err != nil || string(got) != "manual" {
		t.Fatalf("existing documentation changed: %q %v", got, err)
	}
}

func TestOfflineDocumentationDetectionForBothEngines(t *testing.T) {
	for _, tc := range []struct{ kind, exe, language string }{
		{kindUnity, "Unity.exe", "en"},
		{kindTuanjie, "Tuanjie.exe", "zh_CN"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			root := documentationFixtureRoot(t)
			if err := os.WriteFile(filepath.Join(root, "Editor", tc.exe), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
			docs := filepath.Join(root, "Editor", "Data", "Documentation")
			for _, section := range []string{"Manual", "ScriptReference"} {
				file := filepath.Join(docs, tc.language, section, "index.html")
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(section), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			editors := scanEditorsIn([]string{root})
			if len(editors) != 1 || editors[0].Kind != tc.kind || editors[0].OfflineDocs != docs {
				t.Fatalf("documentation not discovered: %+v", editors)
			}
			for _, section := range []string{"Manual", "ScriptReference"} {
				want := filepath.Join(docs, tc.language, section, "index.html")
				if got := offlineDocumentationEntry(editors[0], section); got != want {
					t.Fatalf("%s entry = %q, want %q", section, got, want)
				}
			}
			if err := os.Remove(filepath.Join(docs, tc.language, "Manual", "index.html")); err != nil {
				t.Fatal(err)
			}
			if editors = scanEditorsIn([]string{root}); len(editors) != 1 || editors[0].OfflineDocs != "" {
				t.Fatalf("incomplete documentation advertised: %+v", editors)
			}
		})
	}
}

func TestTuanjieDocumentationRejectsUnsafeOrIncompleteArchives(t *testing.T) {
	for _, files := range []map[string]string{
		{"../outside": "bad"},
		{"Documentation/zh_CN/Manual/index.html": "manual"},
		{"Documentation/zh_CN/Manual/index.html": "manual", "Documentation/zh_CN/ScriptReference/index.html": "api", "other": "bad"},
	} {
		root := documentationFixtureRoot(t)
		archive := makeModuleZip(t, files)
		if err := installTuanjieDocumentation(root, archive, documentationFixtureIntegrity(t, archive)); err == nil {
			t.Fatalf("unsafe archive accepted: %v", files)
		}
		entries, err := os.ReadDir(filepath.Join(root, "Editor", "Data"))
		if err != nil || len(entries) != 0 {
			t.Fatalf("failed install left files: %v %v", entries, err)
		}
	}
}

func TestTuanjieDocumentationRejectsIncorrectDigest(t *testing.T) {
	root := documentationFixtureRoot(t)
	archive := makeModuleZip(t, map[string]string{
		"Documentation/zh_CN/Manual/index.html":          "manual",
		"Documentation/zh_CN/ScriptReference/index.html": "api",
	})
	if err := installTuanjieDocumentation(root, archive, "md5-"+base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Fatal("wrong digest accepted")
	}
	entries, err := os.ReadDir(filepath.Join(root, "Editor", "Data"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed digest left files: %v %v", entries, err)
	}
}

func TestLiveInstalledTuanjieOfflineDocumentation(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_LIVE_TUANJIE_DOCS_EDITOR")
	if exe == "" {
		t.Skip("set UNITYSTARTER_LIVE_TUANJIE_DOCS_EDITOR to an editor with documentation")
	}
	editors := scanEditorsIn([]string{filepath.Dir(exe)})
	if len(editors) != 1 || editors[0].Kind != kindTuanjie || editors[0].OfflineDocs == "" {
		t.Fatalf("installed offline documentation not discovered: %+v", editors)
	}
}

func TestLiveInstalledUnityOfflineDocumentation(t *testing.T) {
	root := os.Getenv("UNITYSTARTER_LIVE_UNITY_MODULES_ROOT")
	if root == "" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_MODULES_ROOT to an editor with documentation")
	}
	editors := scanEditorsIn([]string{root})
	if len(editors) != 1 || editors[0].Kind != kindUnity || editors[0].OfflineDocs == "" {
		t.Fatalf("installed Unity offline documentation not discovered: %+v", editors)
	}
	for _, section := range []string{"Manual", "ScriptReference"} {
		info, err := os.Stat(offlineDocumentationEntry(editors[0], section))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("Unity %s entry missing: %v", section, err)
		}
	}
	for _, m := range readEditorModules(root) {
		if m.ID == "documentation" {
			if !m.Selected || m.PresenceUnverified {
				t.Fatal("installed Unity documentation state not verified")
			}
			return
		}
	}
	t.Fatal("installed Unity documentation module missing")
}

func TestLiveTuanjieDocumentationInstall(t *testing.T) {
	archive := os.Getenv("UNITYSTARTER_LIVE_DOCUMENTATION")
	if archive == "" {
		t.Skip("set UNITYSTARTER_LIVE_DOCUMENTATION to the official archive")
	}
	digest, err := hex.DecodeString("b171e5015bfca969dc13a3d1bd8c667a")
	if err != nil {
		t.Fatal(err)
	}
	root := documentationFixtureRoot(t)
	if err := installTuanjieDocumentation(root, archive, "md5-"+base64.StdEncoding.EncodeToString(digest)); err != nil {
		t.Fatal(err)
	}
	if !documentationFilesPresent(filepath.Join(root, "Editor", "Data", "Documentation")) {
		t.Fatal("official documentation entry points are missing")
	}
}

func TestLiveTuanjieDocumentationDownloadAndInstall(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_DOCS_NETWORK") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_DOCS_NETWORK=1 to download the official archive")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := documentationFixtureRoot(t)
	m := documentationFixtureModule()
	m.Name, m.Visible = "Documentation", true
	m.Checksum, m.DownloadSize = "b171e5015bfca969dc13a3d1bd8c667a", 633122427
	data, err := json.Marshal([]editorModule{m})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Editor", "Tuanjie.exe"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, _, ok := tuanjieDocumentationArchive(m)
	if !ok {
		t.Fatal("official documentation metadata rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	archive := filepath.Join(t.TempDir(), "documentation.zip")
	if err := downloadFileVerifiedSize(ctx, m.URL, archive, sum, m.DownloadSize, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if err := installTuanjieDocumentation(root, archive, sum); err != nil {
		t.Fatal(err)
	}
	if err := markModuleInstalled(root, m); err != nil {
		t.Fatal(err)
	}
	modules := readEditorModules(root)
	editors := scanEditorsIn([]string{root})
	if len(modules) != 1 || !modules[0].Selected || len(editors) != 1 || editors[0].OfflineDocs == "" {
		t.Fatalf("installed documentation was not recorded and discovered: %+v %+v", modules, editors)
	}
}
