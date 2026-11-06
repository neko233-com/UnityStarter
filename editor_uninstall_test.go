package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func uninstallFixture(t *testing.T) editorInstall {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Editor with spaces")
	dir := filepath.Join(root, "Editor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Unity.exe", "Uninstall.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return editorInstall{Root: root, Exe: filepath.Join(dir, "Unity.exe"), Kind: kindUnity, Version: "6000.0.2f1"}
}

func TestUninstallWaitsForRemovalAndRejectsRunningEditor(t *testing.T) {
	e := uninstallFixture(t)
	called := false
	run := func(string) error { called = true; return nil }
	if err := uninstallVerifiedEditor(context.Background(), e, func(string) (bool, error) { return true, nil }, run); err == nil || called {
		t.Fatal("running editor was not protected")
	}
	denied := errors.New("authorization denied")
	if err := uninstallVerifiedEditor(context.Background(), e, func(string) (bool, error) { return false, nil }, func(string) error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("uninstaller error was not propagated: %v", err)
	}
	wrong := e
	wrong.Exe = filepath.Join(t.TempDir(), "Unity.exe")
	if err := uninstallVerifiedEditor(context.Background(), wrong, func(string) (bool, error) { return false, nil }, run); err == nil || called {
		t.Fatal("unrelated executable was accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := uninstallVerifiedEditor(ctx, e, func(string) (bool, error) { return false, nil }, run); !errors.Is(err, errUninstallUnconfirmed) || !called {
		t.Fatalf("uninstaller exit without removal was claimed successful: %v", err)
	}
	if err := uninstallVerifiedEditor(context.Background(), e, func(string) (bool, error) { return false, nil }, func(string) error { return os.Remove(e.Exe) }); err != nil {
		t.Fatalf("confirmed removal was not accepted: %v", err)
	}
}

func TestUninstallProgressRemainsAccessible(t *testing.T) {
	a := fixture()
	a.manageEditor = editorInstall{Exe: "editor", Kind: kindUnity, Version: "6000.0.2f1"}
	a.manageOpen, a.manageUninstall, a.uninstallBusy = true, true, true
	a.uninstallStatus = locales.InstallUninstallProgress()
	tt := ui.NewTester(a.view, 1180, 760)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.manageOpen || !strings.Contains(strings.Join(tt.Texts(), "\n"), locales.InstallUninstallProgress()) {
		t.Fatal("uninstall progress cannot be recovered after dismissal")
	}
	if err := tt.Click(locales.InstallManage()); err != nil {
		t.Fatal(err)
	}
	if !a.manageOpen || !a.manageUninstall || !a.uninstallBusy {
		t.Fatal("uninstall progress reopened the wrong state")
	}
	a.applyUpdateNow()
	if !a.alertOpen || a.alertMessage != locales.InstallUninstallBusy() {
		t.Fatal("application update was not blocked during uninstall")
	}
}

func TestUninstallDialogShowsConfirmation(t *testing.T) {
	a := fixture()
	a.manageEditor = uninstallFixture(t)
	a.manageOpen, a.manageUninstall = true, true
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.InstallUninstallWarning()) || !tt.HasText(locales.InstallUninstallOfficial()) || !tt.HasText(locales.InstallUninstall()) {
		t.Fatalf("uninstall confirmation is incomplete: %q", tt.Texts())
	}
	if path := os.Getenv("UNITYSTARTER_UNINSTALL_SNAPSHOT"); path != "" {
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}
