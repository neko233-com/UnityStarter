package main

import (
	"context"
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
	if !readModuleState(root)["webgl"] {
		t.Fatal("owned state not saved")
	}
	if _, err := os.Stat(filepath.Join(root, "modules.json")); !os.IsNotExist(err) {
		t.Fatal("created incomplete Hub metadata")
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
