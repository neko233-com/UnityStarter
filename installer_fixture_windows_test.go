//go:build windows

package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
			metadata := fmt.Sprintf(`[{"id":"webgl","downloadUrl":%q,"checksum":%q,"selected":false,"keep":42}]`, m.URL, m.Checksum)
			if err := os.WriteFile(filepath.Join(root, "modules.json"), []byte(metadata), 0o644); err != nil {
				t.Fatal(err)
			}
			installer := filepath.Join(t.TempDir(), "module.exe")
			if err := downloadFileVerified(context.Background(), server.URL, installer, "md5-"+base64.StdEncoding.EncodeToString(sum[:]), func(int64, int64) {}); err != nil {
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
