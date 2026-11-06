//go:build windows

package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModuleInstallerFixture(t *testing.T) {
	fixture := os.Getenv("UNITYSTARTER_MODULE_FIXTURE")
	if fixture == "" {
		t.Skip("set UNITYSTARTER_MODULE_FIXTURE to the NSIS fixture executable")
	}
	content, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(content) }))
	defer server.Close()
	for _, kind := range []string{kindUnity, kindTuanjie} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Editor location with spaces")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			m := editorModule{ID: "webgl", URL: "https://download.unity3d.com/web.exe", Checksum: hex.EncodeToString(sum[:]), Destination: "{UNITY_PATH}"}
			if kind == kindUnity {
				m.Integrity = "md5-" + base64.StdEncoding.EncodeToString([]byte(m.Checksum))
				m.Checksum = ""
			}
			metadata, err := json.Marshal([]map[string]any{{"id": m.ID, "downloadUrl": m.URL, "checksum": m.Checksum, "integrity": m.Integrity, "destination": m.Destination, "selected": false, "keep": 42}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "modules.json"), metadata, 0o644); err != nil {
				t.Fatal(err)
			}
			digest, _, _, ok := moduleInstallSpec(editorInstall{Kind: kind}, m)
			if !ok {
				t.Fatal("fixture metadata rejected")
			}
			installer := filepath.Join(t.TempDir(), "module.exe")
			if err := downloadFileVerified(context.Background(), server.URL, installer, digest, func(int64, int64) {}); err != nil {
				t.Fatal(err)
			}
			if err := runEditorInstaller(installer, kind, root); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, "Editor", "Data", "PlaybackEngines", moduleOutput(m), "fixture.txt")); err != nil {
				t.Fatal(err)
			}
			if err := markModuleInstalled(root, m); err != nil {
				t.Fatal(err)
			}
			if modules := readEditorModules(root); len(modules) != 1 || !modules[0].Selected {
				t.Fatal("module state not saved")
			}
		})
	}
}

func TestLiveUnityHubModuleDownloadAndInstall(t *testing.T) {
	source := os.Getenv("UNITYSTARTER_LIVE_UNITY_MODULES_ROOT")
	if source == "" || os.Getenv("UNITYSTARTER_LIVE_UNITY_MODULE_INSTALL") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_MODULES_ROOT and UNITYSTARTER_LIVE_UNITY_MODULE_INSTALL=1")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m editorModule
	for _, item := range readEditorModules(source) {
		if item.ID == "windows-il2cpp" {
			m = item
			break
		}
	}
	sum, _, _, ok := moduleInstallSpec(editorInstall{Kind: kindUnity}, m)
	if !ok {
		t.Fatal("official Hub module metadata rejected")
	}
	m.Selected = false
	root := filepath.Join(t.TempDir(), "Editor with spaces")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Editor"), 0o755); err != nil {
		t.Fatal(err)
	}
	original, err := os.Open(filepath.Join(source, "Editor", "Unity.exe"))
	if err != nil {
		t.Fatal(err)
	}
	copyFile, err := os.Create(filepath.Join(root, "Editor", "Unity.exe"))
	if err != nil {
		original.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(copyFile, original)
	closeErr := copyFile.Close()
	original.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy isolated editor executable: %v %v", copyErr, closeErr)
	}
	metadata, err := json.Marshal([]editorModule{m})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules.json"), metadata, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	installer := filepath.Join(t.TempDir(), "official-module.exe")
	if err := downloadFileVerified(ctx, m.URL, installer, sum, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if err := runEditorInstaller(installer, kindUnity, root); err != nil {
		t.Fatal(err)
	}
	if !moduleFilesPresent(root, m) {
		t.Fatal("official installer did not create the expected module files")
	}
	if err := markModuleInstalled(root, m); err != nil {
		t.Fatal(err)
	}
	if modules := readEditorModules(root); len(modules) != 1 || !modules[0].Selected || modules[0].PresenceUnverified {
		t.Fatalf("official module not verified after install: %+v", modules)
	}
}
