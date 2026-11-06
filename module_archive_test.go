package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jdkFixtureModule() editorModule {
	sum := sha256.Sum256([]byte("fixture"))
	return editorModule{ID: "android-open-jdk-17.0.18+8", Parent: "android", URL: "https://download.unity3d.com/jdk_" + hex.EncodeToString(sum[:]) + ".zip", Destination: "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/OpenJDK"}
}

func TestOpenJDKModuleRequiresTrustedMetadataAndParent(t *testing.T) {
	m := jdkFixtureModule()
	if _, player, ok := openJDKArchive(m); !ok || player != "AndroidPlayer" {
		t.Fatalf("valid module rejected: %+v", m)
	}
	e := editorInstall{Modules: []editorModule{{ID: "android", Selected: false}, m}}
	if _, _, _, ok := moduleInstallSpec(e, m); ok {
		t.Fatal("missing parent accepted")
	}
	e.Modules[0].Selected = true
	if _, _, extension, ok := moduleInstallSpec(e, m); !ok || extension != ".zip" {
		t.Fatal("verified JDK rejected")
	}
	for _, mutate := range []func(*editorModule){
		func(v *editorModule) { v.URL = strings.Replace(v.URL, "download.unity3d.com", "example.com", 1) },
		func(v *editorModule) { v.Destination = "{UNITY_PATH}/../../outside" },
		func(v *editorModule) { v.EULA = "https://example.com/license" },
		func(v *editorModule) { v.URL += "?token=x" },
	} {
		v := m
		mutate(&v)
		if _, _, ok := openJDKArchive(v); ok {
			t.Fatalf("unsafe metadata accepted: %+v", v)
		}
	}
}

func makeModuleZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "module.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range entries {
		out, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeEditorRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func archiveIntegrity(t *testing.T, archive string) string {
	t.Helper()
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

func TestOpenJDKAtomicExtractionAndExistingFiles(t *testing.T) {
	root := fakeEditorRoot(t)
	archive := makeModuleZip(t, map[string]string{"bin/java.exe": "java", "lib/a.txt": "library"})
	if err := installOpenJDK(root, archive, "AndroidPlayer", archiveIntegrity(t, archive)); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer", "OpenJDK")
	if got, err := os.ReadFile(filepath.Join(dest, "bin", "java.exe")); err != nil || string(got) != "java" {
		t.Fatalf("java: %q %v", got, err)
	}
	if !moduleFilesPresent(root, jdkFixtureModule()) {
		t.Fatal("installed JDK not detected")
	}
	if err := installOpenJDK(root, archive, "AndroidPlayer", archiveIntegrity(t, archive)); err == nil {
		t.Fatal("existing JDK overwritten")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "lib", "a.txt")); err != nil || string(got) != "library" {
		t.Fatalf("existing files changed: %q %v", got, err)
	}
}

func TestOpenJDKRejectsUnsafeArchivesWithoutPartialInstall(t *testing.T) {
	for _, entries := range []map[string]string{
		{"../escape.txt": "bad", "bin/java.exe": "java"},
		{"bin/java.exe": "java", "bin/JAVA.exe": "duplicate"},
		{"bin/java.exe": "java", "NUL.txt": "device"},
		{"not-java.txt": "missing"},
	} {
		root := fakeEditorRoot(t)
		archive := makeModuleZip(t, entries)
		if err := installOpenJDK(root, archive, "AndroidPlayer", archiveIntegrity(t, archive)); err == nil {
			t.Fatalf("unsafe archive accepted: %v", entries)
		}
		parent := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
		if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
			t.Fatalf("partial installation remains: %v %v", entries, err)
		}
	}
}

func TestOpenJDKRejectsChangedArchive(t *testing.T) {
	root := fakeEditorRoot(t)
	archive := makeModuleZip(t, map[string]string{"bin/java.exe": "java"})
	if err := installOpenJDK(root, archive, "AndroidPlayer", "sha256-"+base64.StdEncoding.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("archive with wrong digest accepted")
	}
	parent := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("partial installation remains: %v %v", entries, err)
	}
}

func TestLiveOpenJDKArchive(t *testing.T) {
	archive := os.Getenv("UNITYSTARTER_LIVE_JDK_FIXTURE")
	if archive == "" {
		t.Skip("set UNITYSTARTER_LIVE_JDK_FIXTURE to the verified official JDK archive")
	}
	root := fakeEditorRoot(t)
	digest, err := hex.DecodeString("15e8817d1f5db6db3571ebe7430ef37f7fa8e60e8ff6f3e18ca1cb4c29f78774")
	if err != nil {
		t.Fatal(err)
	}
	if err := installOpenJDK(root, archive, "AndroidPlayer", "sha256-"+base64.StdEncoding.EncodeToString(digest)); err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer", "OpenJDK", "bin", "java.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, java, "-version")
	hideWindowCmd(cmd)
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "17.0.18") {
		t.Fatalf("java version: %s %v", out, err)
	}
}
