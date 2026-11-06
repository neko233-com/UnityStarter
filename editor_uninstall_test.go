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
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
	guards := uninstallGuards()
	if len(guards) != 1 {
		t.Fatal("uncertain uninstaller error did not retain protection")
	}
	if err := clearUninstallGuard(guards[0]); err != nil {
		t.Fatal(err)
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
	guards = uninstallGuards()
	if len(guards) != 1 {
		t.Fatal("timed-out uninstall lost protection")
	}
	if err := launchWithEditorLock(e.Exe, nil, "", func(string, []string, string) error { t.Error("protected editor launched"); return nil }); !errors.Is(err, errUninstallProtected) {
		t.Fatalf("unconfirmed uninstall did not block launch: %v", err)
	}
	if err := clearUninstallGuard(guards[0]); err != nil {
		t.Fatal(err)
	}
	if err := uninstallVerifiedEditor(context.Background(), e, func(string) (bool, error) { return false, nil }, func(string) error {
		if err := os.Remove(e.Exe); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(e.Root, "Editor", "Uninstall.exe")); err != nil {
			return err
		}
		return os.Remove(filepath.Join(e.Root, "Editor"))
	}); err != nil {
		t.Fatalf("confirmed removal was not accepted: %v", err)
	}
	if len(uninstallGuards()) != 0 {
		t.Fatal("completed uninstall kept its protection")
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

func TestUninstallProtectionCanBeReviewedAndCleared(t *testing.T) {
	e := uninstallFixture(t)
	if _, err := writeUninstallGuard(e); err != nil {
		t.Fatal(err)
	}
	a := fixture()
	a.uninstallGuards = uninstallGuards()
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.UninstallGuardBanner()) {
		t.Fatal("uninstall protection was not visible")
	}
	for _, label := range []string{locales.UninstallGuardReview(), locales.UninstallGuardRelease(), locales.UninstallGuardConfirm()} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if len(uninstallGuards()) != 0 || a.uninstallGuardOpen {
		t.Fatal("confirmed protection was not cleared")
	}
	if _, err := os.Stat(e.Exe); err != nil {
		t.Fatal("clearing protection deleted the editor")
	}
}

func TestUnreadableUninstallProtectionRemainsRecoverable(t *testing.T) {
	e := uninstallFixture(t)
	path, err := writeUninstallGuard(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	guards := uninstallGuards()
	if len(guards) != 1 || guards[0].Exe != "" {
		t.Fatalf("corrupt record was not surfaced safely: %+v", guards)
	}
	if err := launchWithEditorLock(e.Exe, nil, "", func(string, []string, string) error { return nil }); !errors.Is(err, errUninstallProtected) {
		t.Fatal("corrupt protection did not block launch")
	}
	if err := clearUninstallGuard(guards[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.Exe); err != nil {
		t.Fatal("clearing corrupt protection deleted the editor")
	}
}
