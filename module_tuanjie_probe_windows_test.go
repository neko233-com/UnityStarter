//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Downloads and installs official Tuanjie modules into a disposable editor
// directory; it never writes to the source editor.
func TestLiveTuanjieAdditionalPlatforms(t *testing.T) {
	source := os.Getenv("UNITYSTARTER_LIVE_TUANJIE_MODULES_ROOT")
	ids := os.Getenv("UNITYSTARTER_LIVE_TUANJIE_PLATFORM_MODULES")
	if source == "" || ids == "" {
		t.Skip("set UNITYSTARTER_LIVE_TUANJIE_MODULES_ROOT and UNITYSTARTER_LIVE_TUANJIE_PLATFORM_MODULES")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	version := editorExecutableVersion(filepath.Join(source, "Editor", "Tuanjie.exe"))
	if version == "" {
		t.Fatal("cannot read the source editor version")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	data, err := fetchTuanjieCatalog(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	modules := tuanjieModulesFromCatalog(data, version)
	for _, id := range strings.Split(ids, ",") {
		t.Run(id, func(t *testing.T) {
			var m editorModule
			for _, item := range modules {
				if item.ID == id {
					m = item
					break
				}
			}
			digest, ok := standaloneModuleMD5(m)
			if !ok || !strings.HasPrefix(m.URL, "https://download.tuanjie.cn/") {
				t.Fatal("official module metadata missing")
			}
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "Editor"), 0o755); err != nil {
				t.Fatal(err)
			}
			original, err := os.Open(filepath.Join(source, "Editor", "Tuanjie.exe"))
			if err != nil {
				t.Fatal(err)
			}
			copyFile, err := os.Create(filepath.Join(root, "Editor", "Tuanjie.exe"))
			if err != nil {
				original.Close()
				t.Fatal(err)
			}
			_, copyErr := io.Copy(copyFile, original)
			closeErr := copyFile.Close()
			original.Close()
			if copyErr != nil || closeErr != nil {
				t.Fatalf("copy editor: %v %v", copyErr, closeErr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			installer := filepath.Join(t.TempDir(), "module.exe")
			if err := downloadFileVerified(ctx, m.URL, installer, "md5-"+base64.StdEncoding.EncodeToString(digest), func(int64, int64) {}); err != nil {
				t.Fatal(err)
			}
			if err := runEditorInstaller(installer, kindTuanjie, root); err != nil {
				t.Fatal(err)
			}
			if !standaloneModule(m) || !moduleFilesPresent(root, m) {
				t.Fatal("official platform module files were not recognized")
			}
			if err := markModuleInstalled(root, m); err != nil {
				t.Fatal(err)
			}
			installed := []editorModule{m}
			installed[0].Selected = false
			applyOwnedModuleState(root, installed)
			if !installed[0].Selected {
				t.Fatal("platform module state was not recorded")
			}
		})
	}
}
