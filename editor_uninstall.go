package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unitystarter/locales"
)

var errUninstallUnconfirmed = errors.New("editor directory remains; uninstall completion could not be confirmed")

func uninstallVerifiedEditor(ctx context.Context, e editorInstall, running func(string) (bool, error), run func(string) error) error {
	if e.Simulated || (e.Kind != kindUnity && e.Kind != kindTuanjie) {
		return fmt.Errorf("invalid editor installation")
	}
	name := "Unity.exe"
	if e.Kind == kindTuanjie {
		name = "Tuanjie.exe"
	}
	expected := filepath.Join(e.Root, "Editor", name)
	if !filepath.IsAbs(e.Root) || !strings.EqualFold(filepath.Clean(e.Exe), filepath.Clean(expected)) {
		return fmt.Errorf("invalid editor executable path")
	}
	release, err := acquireEditorLock(e.Exe)
	if err != nil {
		return err
	}
	defer release()
	if info, err := os.Stat(e.Exe); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("editor executable not found")
	}
	active, err := running(e.Exe)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("editor is running; close it before uninstalling")
	}
	uninstaller := editorUninstaller(e)
	if uninstaller == "" {
		return fmt.Errorf("editor uninstaller not found")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	guard, err := writeUninstallGuard(e)
	if err != nil {
		return fmt.Errorf("cannot record uninstall protection: %w", err)
	}
	if err := run(uninstaller); err != nil {
		return err
	}
	// The official uninstaller may return before its temporary child deletes
	// the files. Never report success just from the original process exit.
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := os.Stat(filepath.Join(e.Root, "Editor"))
		if os.IsNotExist(err) {
			if err := os.Remove(guard); err != nil {
				return fmt.Errorf("editor directory removed but protection could not be cleared: %w", err)
			}
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return errUninstallUnconfirmed
		case <-ticker.C:
		}
	}
}

func (a *app) editorWorkActive() bool {
	if a.moduleBusy || a.moduleLoading || a.uninstallBusy || len(a.moduleQueue) > 0 || a.androidPlan != nil {
		return true
	}
	for _, job := range a.jobs {
		if job != nil && (job.Status == "downloading" || job.Status == "downloaded" || job.Status == "installing" || job.Status == "clearing") {
			return true
		}
	}
	return false
}

func (a *app) startEditorUninstall(e editorInstall) {
	if a.win == nil || a.editorWorkActive() {
		return
	}
	a.uninstallBusy, a.uninstallStatus = true, locales.InstallUninstallProgress()
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := uninstallVerifiedEditor(ctx, e, editorProcessRunning, runEditorUninstaller)
		win.Update(func() {
			a.uninstallBusy = false
			if err != nil {
				a.uninstallStatus = locales.InstallUninstallFailed(err.Error())
			} else {
				a.uninstallStatus = locales.InstallUninstallDone()
			}
			a.refresh()
		})
	}()
}
