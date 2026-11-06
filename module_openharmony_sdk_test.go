package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openHarmonySDKFixture() editorModule {
	return editorModule{ID: openHarmonySDKModule, Name: "OpenHarmony SDK", Visible: true, Parent: "openharmony", URL: "https://repo.huaweicloud.com/openharmony/os/6.1-Release/ohos-sdk-windows_linux-public.tar.gz", Destination: "{UNITY_PATH}/Editor/Data/PlaybackEngines/OpenHarmonyPlayer"}
}

func TestOpenHarmonySDKMetadataAndQueue(t *testing.T) {
	m := openHarmonySDKFixture()
	parent := editorModule{ID: "openharmony", Visible: true, URL: "https://download.tuanjie.cn/module.exe", Checksum: strings.Repeat("a", 32), Destination: "{UNITY_PATH}"}
	e := editorInstall{Kind: kindTuanjie, Modules: []editorModule{m, parent}}
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("missing parent accepted")
	}
	queue, err := planModuleQueue(e, map[string]bool{m.ID: true, parent.ID: true})
	if err != nil || !reflect.DeepEqual(queue, []string{parent.ID, m.ID}) {
		t.Fatalf("dependency ordering: %v %v", queue, err)
	}
	e.Modules[1].Selected = true
	if _, _, _, ok := moduleInstallSpec(e, m); !ok {
		t.Fatal("SDK rejected")
	}
	for _, mutate := range []func(*editorModule){func(v *editorModule) { v.URL += "?x=1" }, func(v *editorModule) { v.Destination += "/.." }, func(v *editorModule) { v.EULA = "unknown" }} {
		v := m
		mutate(&v)
		if _, ok := openHarmonySDKArchive(v); ok {
			t.Fatal("unsafe metadata accepted")
		}
	}
}

func makeSDKTar(t *testing.T, zipName string, files map[string]string) string {
	t.Helper()
	component, err := os.ReadFile(makeModuleZip(t, files))
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "sdk.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: zipName, Mode: 0o644, Size: int64(len(component))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(component); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestOpenHarmonySDKAtomicInstall(t *testing.T) {
	for _, tc := range []struct {
		name, path, version string
		success             bool
	}{
		{"valid", "windows/native-windows.zip", `{"apiVersion":23,"version":"6.1.0.31"}`, true},
		{"invalid-version", "windows/native-windows.zip", `{"apiVersion":22,"version":"6.1.0.31"}`, false},
		{"traversal", "../windows/native.zip", `{"apiVersion":23,"version":"6.1.0.31"}`, false},
		{"other-platform", "linux/native.zip", `{"apiVersion":23,"version":"6.1.0.31"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer")
			if err := os.MkdirAll(parent, 0o755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{}
			for _, component := range []string{"native", "ets", "js", "previewer", "toolchains"} {
				files[component+"/oh-uni-package.json"] = tc.version
			}
			files["native/llvm/bin/clang.exe"] = "clang"
			files["ets/build-tools/ets-loader/package.json"] = "{}"
			files["toolchains/hdc.exe"] = "hdc"
			archive := makeSDKTar(t, tc.path, files)
			err := installOpenHarmonySDK(root, archive, archiveIntegrity(t, archive))
			if (err == nil) != tc.success {
				t.Fatalf("install: %v", err)
			}
			if !tc.success {
				if entries, _ := os.ReadDir(parent); len(entries) != 0 {
					t.Fatalf("partial install: %v", entries)
				}
				return
			}
			if !openHarmonySDKFilesPresent(root) {
				t.Fatal("SDK missing")
			}
			if err := installOpenHarmonySDK(root, archive, archiveIntegrity(t, archive)); err == nil {
				t.Fatal("existing SDK overwritten")
			}
			if err := os.Remove(filepath.Join(parent, "SDK", "23", "native", "llvm", "bin", "clang.exe")); err != nil {
				t.Fatal(err)
			}
			if openHarmonySDKFilesPresent(root) {
				t.Fatal("partial SDK reported installed")
			}
		})
	}
}

func TestLiveOpenHarmonySDKInstall(t *testing.T) {
	archive := os.Getenv("UNITYSTARTER_LIVE_OHSDK_ARCHIVE")
	if archive == "" {
		t.Skip("set UNITYSTARTER_LIVE_OHSDK_ARCHIVE to the verified official SDK archive")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum, _ := openHarmonySDKArchive(openHarmonySDKFixture())
	if err := installOpenHarmonySDK(root, archive, sum); err != nil {
		t.Fatal(err)
	}
	if !openHarmonySDKFilesPresent(root) {
		t.Fatal("official SDK not detected")
	}
	entries, err := os.ReadDir(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer", "SDK", "23"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Log(fmt.Sprintf("SDK component: %s", entry.Name()))
	}
}

func TestLiveOpenHarmonySDKDownload(t *testing.T) {
	archive := os.Getenv("UNITYSTARTER_LIVE_OHSDK_DOWNLOAD")
	if archive == "" {
		t.Skip("set UNITYSTARTER_LIVE_OHSDK_DOWNLOAD to an isolated download path")
	}
	sum, _ := hex.DecodeString("b833b75a64ee46bbd7880921abbb49b733ec5c8171b6684c9b524d57f624cee0")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	err := downloadFileVerifiedSize(ctx, "https://repo.huaweicloud.com/openharmony/os/6.1-Release/ohos-sdk-windows_linux-public.tar.gz", archive, "sha256-"+base64.StdEncoding.EncodeToString(sum), 2499936800, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
}
