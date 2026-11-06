//go:build windows

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This optional test uses official installers in isolated temporary roots.
func TestLiveUnityAdditionalPlatforms(t *testing.T) {
	source := os.Getenv("UNITYSTARTER_LIVE_UNITY_MODULES_ROOT")
	ids := os.Getenv("UNITYSTARTER_LIVE_PLATFORM_MODULES")
	if source == "" || ids == "" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_MODULES_ROOT and UNITYSTARTER_LIVE_PLATFORM_MODULES")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	modules := readEditorModules(source)
	version := editorExecutableVersion(filepath.Join(source, "Editor", "Unity.exe"))
	if version == "" {
		t.Fatal("cannot read the source editor version")
	}
	if len(modules) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		modules, err = fetchUnityModules(ctx, version)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range strings.Split(ids, ",") {
		t.Run(id, func(t *testing.T) {
			var m editorModule
			for _, item := range modules {
				if item.ID == id {
					m = item
					break
				}
			}
			integrity, _, _, valid := moduleInstallSpec(editorInstall{Kind: kindUnity, Version: version}, m)
			if !valid || !strings.HasPrefix(m.URL, "https://download.unity3d.com/") {
				t.Fatal("official module metadata missing")
			}
			root := t.TempDir()
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
				t.Fatalf("copy editor: %v %v", copyErr, closeErr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			installer := filepath.Join(t.TempDir(), "module.exe")
			if err := downloadFileVerified(ctx, m.URL, installer, integrity, func(int64, int64) {}); err != nil {
				t.Fatal(err)
			}
			if err := runEditorInstaller(installer, kindUnity, root); err != nil {
				t.Fatal(err)
			}
			if !standaloneModule(m) || !moduleFilesPresent(root, m) {
				base := filepath.Join(root, "Editor", "Data", "PlaybackEngines")
				if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					rel, _ := filepath.Rel(base, path)
					parts := strings.Split(rel, string(os.PathSeparator))
					if len(parts) <= 4 && (entry.IsDir() || strings.EqualFold(filepath.Ext(path), ".dll") || strings.EqualFold(filepath.Ext(path), ".so") || strings.EqualFold(filepath.Ext(path), ".exe")) {
						t.Logf("installed: %s", rel)
					}
					return nil
				}); err != nil {
					t.Logf("inspect module files: %v", err)
				}
				t.Fatal("official platform module files were not recognized")
			}
			if err := markModuleInstalled(root, m); err != nil {
				t.Fatal(err)
			}
			m.Selected = false
			installed := []editorModule{m}
			applyOwnedModuleState(root, installed)
			if !installed[0].Selected {
				t.Fatal("platform module state was not recorded")
			}
		})
	}
}
