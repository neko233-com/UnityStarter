//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRealUninstallerFixture(t *testing.T) {
	installer := os.Getenv("UNITYSTARTER_UNINSTALL_FIXTURE")
	if installer == "" {
		t.Skip("set UNITYSTARTER_UNINSTALL_FIXTURE to the isolated NSIS fixture")
	}
	root := filepath.Join(t.TempDir(), "installation with spaces")
	if err := runEditorInstaller(installer, kindUnity, root); err != nil {
		t.Fatal(err)
	}
	e := editorInstall{Root: root, Exe: filepath.Join(root, "Editor", "Unity.exe"), Kind: kindUnity, Version: "6000.0.2f1"}
	keep := filepath.Join(root, "Editor", "user-data.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := uninstallVerifiedEditor(ctx, e, editorProcessRunning, runEditorUninstaller); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.Exe); !os.IsNotExist(err) {
		t.Fatal("fixture editor still present")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("unrelated file was removed")
	}
}

func TestLiveRunningEditorDetection(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_RUNNING_EDITOR")
	if exe == "" {
		t.Skip("set UNITYSTARTER_RUNNING_EDITOR to a currently running editor executable")
	}
	running, err := editorProcessRunning(exe)
	if err != nil || !running {
		t.Fatalf("running editor not detected: running=%v err=%v", running, err)
	}
}
