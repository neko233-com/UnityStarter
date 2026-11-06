package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnityModuleCatalogAndOwnedState(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") != "7000.0.0a16" {
			t.Errorf("version query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"results":[{"version":"7000.0.0a16","downloads":[{"platform":"WINDOWS","architecture":"X86_64","modules":[{"id":"webgl","name":"Web","url":"https://download.unity3d.com/web.exe","integrity":"md5-ASNFZ4mrze8BI0VniavN7w==","destination":"{UNITY_PATH}","subModules":[{"id":"child","name":"Child","hidden":false}]}]}]}]}`))
	}))
	defer server.Close()
	previous := unityModuleCatalogURL
	unityModuleCatalogURL = server.URL + "?version="
	defer func() { unityModuleCatalogURL = previous }()
	modules, err := fetchUnityModules(context.Background(), "7000.0.0a16")
	if err != nil || len(modules) != 2 || !standaloneModule(modules[0]) || modules[1].Parent != "webgl" || standaloneModule(modules[1]) {
		t.Fatalf("catalog: %+v %v", modules, err)
	}
	root := t.TempDir()
	if err := markModuleInstalled(root, modules[0]); err != nil {
		t.Fatal(err)
	}
	if !readModuleState(root)["webgl"].Installed {
		t.Fatal("owned state not saved")
	}
	if _, err := os.Stat(filepath.Join(root, "modules.json")); !os.IsNotExist(err) {
		t.Fatal("created incomplete Hub metadata")
	}
}

func TestLegacyUnityModuleIntegrity(t *testing.T) {
	const digest = "6eab785d35754b9b0145e60237c8b192"
	var modules []editorModule
	appendUnityModules(&modules, []unityModule{{ID: "webgl", URL: "https://download.unity3d.com/web.exe", Integrity: "md5-" + base64.StdEncoding.EncodeToString([]byte(digest)), Destination: "{UNITY_PATH}"}}, "")
	if len(modules) != 1 || modules[0].Checksum != digest || !standaloneModule(modules[0]) {
		t.Fatalf("legacy module metadata rejected: %+v", modules)
	}
}

func TestHubModuleIntegrityWithoutChecksum(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const digest = "6eab785d35754b9b0145e60237c8b192"
	m := editorModule{ID: "webgl", URL: "https://download.unity3d.com/web.exe", Integrity: "md5-" + base64.StdEncoding.EncodeToString([]byte(digest)), Destination: "{UNITY_PATH}"}
	if sum, _, _, ok := moduleInstallSpec(editorInstall{Kind: kindUnity}, m); !ok || sum != "md5-bqt4XTV1S5sBReYCN8ixkg==" {
		t.Fatalf("Hub integrity not recognized: %q %v", sum, ok)
	}
	modern := m
	modern.Integrity = "md5-bqt4XTV1S5sBReYCN8ixkg=="
	modern.Checksum = digest
	if _, _, _, ok := moduleInstallSpec(editorInstall{Kind: kindUnity}, modern); !ok {
		t.Fatal("matching raw MD5 and hexadecimal checksum rejected")
	}
	bad := m
	bad.Checksum = strings.Repeat("0", 32)
	if standaloneModule(bad) {
		t.Fatal("conflicting checksums accepted")
	}
	bad = m
	bad.Integrity = "md5-invalid"
	if standaloneModule(bad) {
		t.Fatal("invalid integrity accepted")
	}
	root := t.TempDir()
	data := []byte(`[{"id":"webgl","downloadUrl":"https://download.unity3d.com/web.exe","integrity":"` + m.Integrity + `","destination":"{UNITY_PATH}","selected":false,"unknown":{"keep":42}}]`)
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	modules := readEditorModules(root)
	if len(modules) != 1 || !standaloneModule(modules[0]) {
		t.Fatalf("Hub metadata rejected: %+v", modules)
	}
	if err := markModuleInstalled(root, modules[0]); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	result, err := os.ReadFile(filepath.Join(root, "modules.json"))
	if err != nil || json.Unmarshal(result, &rows) != nil || rows[0]["selected"] != true || rows[0]["unknown"].(map[string]any)["keep"] != float64(42) {
		t.Fatalf("Hub metadata not preserved: %s %v", result, err)
	}
	if moduleFingerprint(m) == moduleFingerprint(bad) {
		t.Fatal("integrity changes did not change local state identity")
	}
}

func TestLiveUnityHubStandaloneMetadata(t *testing.T) {
	root := os.Getenv("UNITYSTARTER_LIVE_UNITY_MODULES_ROOT")
	if root == "" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_MODULES_ROOT to an installed editor")
	}
	modules := readEditorModules(root)
	for _, id := range []string{"android", "webgl", "windows-il2cpp"} {
		found := false
		for _, m := range modules {
			if m.ID == id {
				found = true
				if _, _, _, ok := moduleInstallSpec(editorInstall{Kind: kindUnity}, m); !ok {
					t.Fatalf("Hub module %s rejected: %+v", id, m)
				}
			}
		}
		if !found {
			t.Fatalf("Hub module %s missing", id)
		}
	}
}

func TestLinuxIL2CPPModuleRequiresBothPlayers(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := editorModule{ID: "linux-il2cpp", URL: "https://download.unity3d.com/linux.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
	if !standaloneModule(m) {
		t.Fatal("verified Linux IL2CPP installer is not supported")
	}
	root := t.TempDir()
	stale := m
	stale.Selected = true
	data, err := json.Marshal([]editorModule{stale})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readEditorModules(root); len(got) != 1 || got[0].Selected {
		t.Fatalf("stale Hub state reported as installed: %+v", got)
	}
	if err := markOwnedModuleInstalled(root, m); err != nil {
		t.Fatal(err)
	}
	modules := []editorModule{m}
	applyOwnedModuleState(root, modules)
	if modules[0].Selected {
		t.Fatal("missing module reported as installed")
	}
	base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "LinuxStandaloneSupport", "Variations")
	for i, variant := range []string{"linux64_player_development_il2cpp", "linux64_player_nondevelopment_il2cpp"} {
		dir := filepath.Join(base, variant)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "UnityPlayer.so"), []byte("player"), 0o644); err != nil {
			t.Fatal(err)
		}
		modules = []editorModule{m}
		applyOwnedModuleState(root, modules)
		if modules[0].Selected != (i == 1) {
			t.Fatalf("installed status after player %d: %+v", i, modules[0])
		}
	}
}

func TestAdditionalPlatformModulesRequireAllPlayerVariants(t *testing.T) {
	for _, id := range []string{"linux-mono", "linux-coreclr", "linux-server", "windows-server", "mac-coreclr", "mac-server-coreclr", "mac-mono", "mac-server", "armlinux"} {
		t.Run(id, func(t *testing.T) {
			m := editorModule{ID: id, URL: "https://download.unity3d.com/module.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
			if !standaloneModule(m) {
				t.Fatal("official standalone module metadata rejected")
			}
			root := t.TempDir()
			variants, player := modulePlayerVariants(id)
			base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", filepath.Dir(filepath.Dir(moduleOutput(m))), "Variations")
			for i, variant := range variants {
				if moduleFilesPresent(root, m) {
					t.Fatalf("partial module accepted after %d of %d files", i, len(variants))
				}
				dir := filepath.Dir(filepath.Join(base, variant, player))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(base, variant, player), []byte("fixture"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if !moduleFilesPresent(root, m) {
				t.Fatal("complete module was not recognized")
			}
		})
	}
}

func TestAppleUWPAndTuanjieModuleFileChecks(t *testing.T) {
	for _, tc := range []struct {
		id    string
		files []string
	}{
		{"appletv", []string{"AppleTVSupport/UnityEditor.AppleTV.Extensions.dll", "AppleTVSupport/Variations/il2cpp/Releasearm64_managed/fixture"}},
		{"visionos", []string{"VisionOSPlayer/UnityEditor.VisionOS.Extensions.dll", "VisionOSPlayer/Variations/il2cpp/Releasearm64_managed/fixture"}},
		{"universal-windows-platform", []string{"MetroSupport/UnityEditor.UWP.Extensions.dll", "MetroSupport/Players/UAP/il2cpp/x64/debug/UnityPlayer.dll", "MetroSupport/Players/UAP/il2cpp/x64/release/UnityPlayer.dll", "MetroSupport/Players/UAP/il2cpp/x64/master/UnityPlayer.dll"}},
		{"openharmony", []string{"OpenHarmonyPlayer/UnityEditor.OpenHarmony.Extensions.dll", "OpenHarmonyPlayer/Variations/il2cpp/Development/Libs/arm64-v8a/libtuanjie.so", "OpenHarmonyPlayer/Variations/il2cpp/Release/Libs/arm64-v8a/libtuanjie.so"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			m := editorModule{ID: tc.id, URL: "https://download.unity3d.com/module.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
			if !standaloneModule(m) {
				t.Fatal("module metadata rejected")
			}
			base := filepath.Join(t.TempDir(), "Editor", "Data", "PlaybackEngines")
			root := filepath.Dir(filepath.Dir(filepath.Dir(base)))
			for i, rel := range tc.files {
				if moduleFilesPresent(root, m) {
					t.Fatalf("partial module accepted at %d", i)
				}
				file := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("fixture"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if !moduleFilesPresent(root, m) {
				t.Fatal("complete module was not detected")
			}
		})
	}
}

func TestAppleModulesRecognizeUnity7Layout(t *testing.T) {
	for _, tc := range []struct{ id, dir, extension, framework string }{
		{"appletv", "AppleTVSupport", "UnityEditor.AppleTV.Extensions.dll", "UnityRuntime-tvOS"},
		{"visionos", "VisionOSPlayer", "UnityEditor.VisionOS.Extensions.dll", "UnityRuntime-il2cpp"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", tc.dir)
			for i, rel := range []string{tc.extension, "Variations/il2cpp/Managed/fixture", "Trampoline/Frameworks/" + tc.framework + "/fixture"} {
				if moduleFilesPresent(root, editorModule{ID: tc.id}) {
					t.Fatalf("incomplete Unity 7 module accepted at %d", i)
				}
				file := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("fixture"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if !moduleFilesPresent(root, editorModule{ID: tc.id}) {
				t.Fatal("Unity 7 Apple module not recognized")
			}
		})
	}
}

func TestServerModulesRecognizeCoreCLR(t *testing.T) {
	for _, id := range []string{"linux-server", "windows-server"} {
		t.Run(id, func(t *testing.T) {
			m := editorModule{ID: id}
			root := t.TempDir()
			variants, player := modulePlayerVariants(id)
			base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", filepath.Dir(filepath.Dir(moduleOutput(m))), "Variations")
			for i, variant := range variants {
				variant = strings.Replace(variant, "_mono", "_coreclr", 1)
				if moduleFilesPresent(root, m) {
					t.Fatalf("partial CoreCLR module accepted at %d", i)
				}
				dir := filepath.Join(base, variant)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, player), []byte("fixture"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if !moduleFilesPresent(root, m) {
				t.Fatal("complete CoreCLR module not recognized")
			}
			if err := os.Remove(filepath.Join(base, strings.Replace(variants[0], "_mono", "_coreclr", 1), player)); err != nil {
				t.Fatal(err)
			}
			mixed := filepath.Join(base, variants[0])
			if err := os.MkdirAll(mixed, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(mixed, player), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
			if moduleFilesPresent(root, m) {
				t.Fatal("mixed Mono and CoreCLR variants were accepted")
			}
		})
	}
}

func TestLiveLinuxIL2CPPModule(t *testing.T) {
	root := os.Getenv("UNITYSTARTER_LINUX_MODULE_ROOT")
	if root == "" {
		t.Skip("set UNITYSTARTER_LINUX_MODULE_ROOT to an isolated official module installation")
	}
	modules, err := fetchUnityModules(context.Background(), "7000.0.0a16")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range modules {
		if m.ID == "linux-il2cpp" && standaloneModule(m) && moduleFilesPresent(root, m) {
			return
		}
	}
	t.Fatal("official Linux IL2CPP module metadata or installed files not recognized")
}

func TestUnityModuleCatalogCachedForOfflineScan(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"version":"7000.0.0a16","downloads":[{"platform":"WINDOWS","architecture":"X86_64","modules":[{"id":"webgl","name":"Web","url":"https://download.unity3d.com/web.exe","integrity":"md5-ASNFZ4mrze8BI0VniavN7w==","destination":"{UNITY_PATH}"}]}]}]}`))
	}))
	previous := unityModuleCatalogURL
	unityModuleCatalogURL = server.URL + "?version="
	defer func() { unityModuleCatalogURL = previous }()
	root := filepath.Join(t.TempDir(), "7000.0.0a16")
	exe := filepath.Join(root, "Editor", "Unity.exe")
	output := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "WebGLSupport")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "fixture"), []byte("module"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := editorInstall{Kind: kindUnity, Version: "7000.0.0a16", Root: root}
	modules, cached, err := moduleCatalogWithSource(context.Background(), e)
	if err != nil || cached || len(modules) != 1 {
		t.Fatalf("online catalog: %+v %v %v", modules, cached, err)
	}
	if err := markOwnedModuleInstalled(root, modules[0]); err != nil {
		t.Fatal(err)
	}
	server.Close()
	modules, cached, err = moduleCatalogWithSource(context.Background(), e)
	if err != nil || !cached || len(modules) != 1 || !modules[0].Selected {
		t.Fatalf("offline catalog and installed state: %+v %v %v", modules, cached, err)
	}
	editors := scanEditorsIn([]string{root})
	if len(editors) != 1 || !editors[0].ModulesCached || len(installedModuleNames(editors[0].Modules)) != 1 {
		t.Fatalf("offline installation scan: %+v", editors)
	}
	if err := os.Remove(filepath.Join(output, "fixture")); err != nil {
		t.Fatal(err)
	}
	modules, _, err = moduleCatalogWithSource(context.Background(), e)
	if err != nil || modules[0].Selected {
		t.Fatalf("missing module files marked installed: %+v %v", modules, err)
	}
	if _, _, err := moduleCatalogWithSource(context.Background(), editorInstall{Kind: kindUnity, Version: "7000.0.0a17", Root: root}); err == nil {
		t.Fatal("another version reused the cached catalog")
	}
	if err := os.WriteFile(unityModuleCachePath(e.Version), []byte(`{"schema":1,"version":"wrong","modules":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := moduleCatalogWithSource(context.Background(), e); err == nil {
		t.Fatal("corrupt or mismatched cache was accepted")
	}
}

func TestModuleMetadataValidationAndPreservation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "modules.json")
	data := `[{"id":"webgl","name":"Web","visible":true,"selected":false,"destination":"{UNITY_PATH}","downloadUrl":"https://download.tuanjie.cn/release/web.exe","checksum":"0123456789abcdef0123456789abcdef","unknown":{"keep":42}},{"id":"android-open-jdk","selected":false}]`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	modules := readEditorModules(root)
	if len(modules) != 2 || !standaloneModule(modules[0]) || standaloneModule(modules[1]) {
		t.Fatalf("unexpected modules: %+v", modules)
	}
	bad := modules[0]
	bad.URL = "https://evil.invalid/web.exe"
	if standaloneModule(bad) {
		t.Fatal("untrusted module accepted")
	}
	bad = modules[0]
	bad.EULA = "https://example.com/license"
	if standaloneModule(bad) {
		t.Fatal("module with extra license accepted")
	}
	bad = modules[0]
	bad.ID = "../webgl"
	if standaloneModule(bad) {
		t.Fatal("unsafe id accepted")
	}
	stale := modules[0]
	stale.Checksum = strings.Repeat("0", 32)
	if err := markModuleInstalled(root, stale); err == nil {
		t.Fatal("changed metadata accepted")
	}
	if err := markModuleInstalled(root, modules[0]); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["selected"] != true || rows[1]["selected"] != false || rows[0]["unknown"].(map[string]any)["keep"] != float64(42) {
		t.Fatalf("metadata lost: %s", out)
	}
}

func TestOwnedModuleStateRequiresMatchingMetadataAndFiles(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	m := editorModule{ID: "webgl", Name: "Web", URL: "https://download.unity3d.com/a.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
	data, err := json.Marshal([]editorModule{m})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := markOwnedModuleInstalled(root, m); err != nil {
		t.Fatal(err)
	}
	if got := readEditorModules(root); len(got) != 1 || got[0].Selected {
		t.Fatalf("missing files marked installed: %+v", got)
	}
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "WebGLSupport"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "WebGLSupport", "fixture"), []byte("module"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readEditorModules(root); len(got) != 1 || !got[0].Selected {
		t.Fatalf("owned state not restored: %+v", got)
	}
	m.URL = "https://download.unity3d.com/changed.exe"
	data, _ = json.Marshal([]editorModule{m})
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readEditorModules(root); len(got) != 1 || got[0].Selected {
		t.Fatalf("changed metadata accepted: %+v", got)
	}
}

func TestHubModuleStateChecksKnownFilesAndLabelsUnknown(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	modules := []editorModule{
		{ID: "webgl", Name: "Web", Selected: true, Visible: true},
		{ID: "vendor-unknown", Name: "Vendor module", Selected: true, Visible: true},
	}
	data, err := json.Marshal(modules)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got := readEditorModules(root)
	if len(got) != 2 || got[0].Selected || !got[1].Selected || !got[1].PresenceUnverified {
		t.Fatalf("missing files or unknown Hub state misrepresented: %+v", got)
	}
	output := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "WebGLSupport")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := readEditorModules(root); got[0].Selected {
		t.Fatal("empty module directory reported installed")
	}
	if err := os.WriteFile(filepath.Join(output, "fixture"), []byte("module"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = readEditorModules(root)
	if !got[0].Selected || got[0].PresenceUnverified {
		t.Fatalf("present module was not recognized: %+v", got)
	}
}

func TestLiveTuanjieModuleFileChecks(t *testing.T) {
	root := os.Getenv("UNITYSTARTER_TEST_TUANJIE_EDITOR")
	if root == "" {
		t.Skip("set UNITYSTARTER_TEST_TUANJIE_EDITOR to inspect an installed editor")
	}
	modules := readEditorModules(root)
	for _, id := range []string{"android", "ios", "windows-il2cpp", "weixinminigame", "android-sdk-ndk-tools", "android-open-jdk"} {
		found := false
		for _, module := range modules {
			if module.ID == id {
				found = module.Selected && !module.PresenceUnverified
				break
			}
		}
		if !found {
			t.Fatalf("known installed module %s not verified", id)
		}
	}
}

func TestInstalledEditorFilters(t *testing.T) {
	a := newApp()
	a.editors = []editorInstall{
		{Version: "6000.0.0a16", Kind: kindUnity, Exe: "alpha"},
		{Version: "6000.0.1f1", Kind: kindUnity, Exe: "stable"},
		{Version: "2022.3.62t17", DisplayVersion: "1.11.0", Kind: kindTuanjie, Exe: "tuanjie"},
	}
	a.installFilter = "preview"
	if got := a.filteredEditors(kindUnity); len(got) != 1 || got[0].Exe != "alpha" {
		t.Fatalf("preview: %+v", got)
	}
	a.installFilter = "official"
	if got := a.filteredEditors(kindUnity); len(got) != 1 || got[0].Exe != "stable" {
		t.Fatalf("official: %+v", got)
	}
	a.installQuery = "1.11.0"
	if got := a.filteredEditors(kindTuanjie); len(got) != 1 || got[0].Exe != "tuanjie" {
		t.Fatalf("Tuanjie display version: %+v", got)
	}
}

func TestTuanjieVersionMapping(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "TuanjieHub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "versionMapping.json"), []byte(`{"2022.3.62t17":"1.11.0","beta":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readTuanjieVersionMap([]string{dir})
	if got["2022.3.62t17"] != "1.11.0" || got["beta"] != "" {
		t.Fatalf("mapping: %+v", got)
	}
}

func TestLiveUnity7ModuleCatalog(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_CATALOG") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_CATALOG=1 to check the official service")
	}
	modules, err := fetchUnityModules(context.Background(), "7000.0.0a16")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range modules {
		if m.ID == "webgl" && standaloneModule(m) {
			return
		}
	}
	t.Fatal("the official Unity 7 catalog has no supported Web module")
}

func TestLiveUnity7ModuleCatalogOfflineCache(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_CATALOG") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_CATALOG=1 to check the official service")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e := editorInstall{Kind: kindUnity, Version: "7000.0.0a16", Root: t.TempDir()}
	online, cached, err := moduleCatalogWithSource(context.Background(), e)
	if err != nil || cached || len(online) == 0 {
		t.Fatalf("online catalog: %d cached=%v err=%v", len(online), cached, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	offline, cached, err := moduleCatalogWithSource(ctx, e)
	if err != nil || !cached || len(offline) != len(online) {
		t.Fatalf("official catalog offline cache: %d / %d cached=%v err=%v", len(offline), len(online), cached, err)
	}
	for i := range online {
		if online[i].ID != offline[i].ID || online[i].DependencyFingerprint != offline[i].DependencyFingerprint {
			t.Fatalf("cached module differs from official metadata: %+v / %+v", online[i], offline[i])
		}
	}
}
