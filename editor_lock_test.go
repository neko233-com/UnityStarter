package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEditorLockHelper(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LOCK_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	release, err := acquireEditorLock(os.Getenv("UNITYSTARTER_LOCK_EDITOR"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := os.WriteFile(os.Getenv("UNITYSTARTER_LOCK_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if _, err := os.Stat(os.Getenv("UNITYSTARTER_LOCK_RELEASE")); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("lock helper timed out")
}

func TestEditorLockBlocksLaunchAcrossProcesses(t *testing.T) {
	e := uninstallFixture(t)
	dir := t.TempDir()
	ready, done := filepath.Join(dir, "ready"), filepath.Join(dir, "release")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, self, "-test.run=^TestEditorLockHelper$")
	child.Env = append(os.Environ(), "UNITYSTARTER_LOCK_HELPER=1", "UNITYSTARTER_LOCK_EDITOR="+e.Exe, "UNITYSTARTER_LOCK_READY="+ready, "UNITYSTARTER_LOCK_RELEASE="+done)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatal("lock holder did not start")
	}
	launched := false
	launch := func(string, []string, string) error { launched = true; return nil }
	if err := launchWithEditorLock(e.Exe, nil, "", launch); err == nil || launched {
		t.Fatal("launch was allowed while another process held the editor lock")
	}
	if err := os.WriteFile(done, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("lock holder failed: %v", err)
	}
	if err := launchWithEditorLock(e.Exe, nil, "", launch); err != nil || !launched {
		t.Fatalf("launch did not resume after release: %v", err)
	}
}

func TestUninstallHoldsEditorLockUntilRemoval(t *testing.T) {
	e := uninstallFixture(t)
	started, continueUninstall := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- uninstallVerifiedEditor(context.Background(), e, func(string) (bool, error) { return false, nil }, func(string) error {
			close(started)
			<-continueUninstall
			return os.Remove(e.Exe)
		})
	}()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("uninstall could not start: %v", err)
	}
	launched := false
	if err := launchWithEditorLock(e.Exe, nil, "", func(string, []string, string) error { launched = true; return nil }); err == nil || launched {
		t.Error("launch was not blocked during uninstall")
	}
	close(continueUninstall)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestEditorInstallDoesNotWriteWhileEditorLocked(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "6000.0.2f1")
	exe := filepath.Join(dest, "Editor", "Unity.exe")
	held, releaseLock := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		release, err := acquireEditorLock(exe)
		if err != nil {
			result <- err
			return
		}
		close(held)
		<-releaseLock
		release()
		result <- nil
	}()
	select {
	case <-held:
	case err := <-result:
		t.Fatalf("could not acquire editor lock: %v", err)
	}
	body := []byte("installer fixture")
	entry := editorInstallFixture(body)
	installer := filepath.Join(base, "installer.exe")
	if err := os.WriteFile(installer, body, 0o600); err != nil {
		close(releaseLock)
		t.Fatal(err)
	}
	called := false
	err := installVerifiedEditorWith(entry, installer, dest, func(string, string, string) error { called = true; return nil }, func(string, string, string) error { return nil })
	if err == nil || called {
		t.Error("installer ran while the editor directory was locked")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("locked installation created a destination")
	}
	close(releaseLock)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestBuiltCLICannotOpenLockedEditor(t *testing.T) {
	app := os.Getenv("UNITYSTARTER_TEST_APP")
	if app == "" {
		t.Skip("set UNITYSTARTER_TEST_APP to the built executable")
	}
	base := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(base, "roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(base, "local"))
	t.Setenv("USERPROFILE", base)
	t.Setenv("ProgramFiles", "")
	t.Setenv("ProgramFiles(x86)", "")
	root := filepath.Join(base, "6000.0.2f1")
	exe := filepath.Join(root, "Editor", "Unity.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "Game")
	writeProject(t, project, "6000.0.2f1")
	if err := savePrefs(prefs{EditorRoots: []string{root}, PinnedProjects: []string{project}}); err != nil {
		t.Fatal(err)
	}
	ready, done := filepath.Join(base, "ready"), filepath.Join(base, "release")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, self, "-test.run=^TestEditorLockHelper$")
	child.Env = append(os.Environ(), "UNITYSTARTER_LOCK_HELPER=1", "UNITYSTARTER_LOCK_EDITOR="+exe, "UNITYSTARTER_LOCK_READY="+ready, "UNITYSTARTER_LOCK_RELEASE="+done)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatal("lock holder did not start")
	}
	output, err := exec.CommandContext(ctx, app, "open", project).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "editor is busy in another UnityStarter operation") {
		t.Fatalf("CLI did not reject the locked editor: %v, %s", err, output)
	}
	if err := os.WriteFile(done, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
}
