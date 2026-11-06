package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

var errUninstallProtected = errors.New("uninstall status is unconfirmed; review the protection in UnityStarter")

type uninstallGuard struct {
	Exe       string    `json:"exe"`
	Version   string    `json:"version"`
	Kind      string    `json:"kind"`
	StartedAt time.Time `json:"startedAt"`
	file      string
	id        string
}

func uninstallGuardPath(exe string) (string, error) {
	id, err := editorLockID(exe)
	if err != nil {
		return "", err
	}
	return filepath.Join(userDataDir(), "uninstall-protection", id+".json"), nil
}

// Called while holding the editor mutex, before launching the uninstaller.
func writeUninstallGuard(e editorInstall) (string, error) {
	path, err := uninstallGuardPath(e.Exe)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(uninstallGuard{Exe: e.Exe, Version: e.Version, Kind: e.Kind, StartedAt: time.Now()})
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func uninstallGuards() []uninstallGuard {
	dir := filepath.Join(userDataDir(), "uninstall-protection")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var guards []uninstallGuard
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		guard := uninstallGuard{file: path, id: id}
		if info.Mode().IsRegular() && info.Size() <= 16384 {
			data, readErr := os.ReadFile(path)
			var decoded uninstallGuard
			if readErr == nil && json.Unmarshal(data, &decoded) == nil && filepath.IsAbs(decoded.Exe) && (decoded.Kind == kindUnity || decoded.Kind == kindTuanjie) {
				expected, pathErr := uninstallGuardPath(decoded.Exe)
				if pathErr == nil && strings.EqualFold(expected, path) {
					guard.Exe, guard.Version, guard.Kind, guard.StartedAt = decoded.Exe, decoded.Version, decoded.Kind, decoded.StartedAt
				}
			}
		}
		guards = append(guards, guard)
	}
	return guards
}

// This only removes our protection record, never the editor or its files.
func clearUninstallGuard(g uninstallGuard) error {
	if len(g.id) != 64 || strings.Trim(g.id, "0123456789abcdef") != "" {
		return fmt.Errorf("invalid uninstall protection record")
	}
	path := filepath.Join(userDataDir(), "uninstall-protection", g.id+".json")
	if !strings.EqualFold(filepath.Clean(path), filepath.Clean(g.file)) {
		return fmt.Errorf("invalid uninstall protection record")
	}
	release, err := acquireDownloadLock("editor-" + g.id)
	if err != nil {
		return err
	}
	defer release()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (a *app) uninstallGuardBanner(c *ui.Context) {
	if a.uninstallBusy || len(a.uninstallGuards) == 0 {
		return
	}
	ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, locales.UninstallGuardBanner()).Grow(1).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Button(c, locales.UninstallGuardReview()).OnClick(func() { a.uninstallGuardOpen, a.uninstallGuardConfirm = true, false })
	})
}

func (a *app) uninstallGuardDialog(c *ui.Context) {
	if !a.uninstallGuardOpen {
		return
	}
	ui.Modal(c, &a.uninstallGuardOpen, func() {
		ui.Column(c).Width(560).Gap(12).Children(func() {
			ui.Text(c, locales.UninstallGuardTitle()).FontSize(18).Bold()
			ui.Text(c, locales.UninstallGuardExplanation()).FontSize(12).TextColor(c.Theme().TextMuted)
			if a.uninstallGuardConfirm {
				g := a.uninstallGuardSelected
				label := g.Exe
				if label == "" {
					label = locales.UninstallGuardUnreadable() + " · " + g.file
				}
				ui.Text(c, label).FontSize(12).SingleLine().Clip().Tooltip(label)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					ui.Button(c, locales.ActionCancel()).OnClick(func() { a.uninstallGuardConfirm = false })
					ui.PrimaryButton(c, locales.UninstallGuardConfirm()).OnClick(func() {
						if err := clearUninstallGuard(g); err != nil {
							a.alert(err.Error())
							return
						}
						a.uninstallGuards = uninstallGuards()
						a.uninstallGuardConfirm = false
						if len(a.uninstallGuards) == 0 {
							a.uninstallGuardOpen = false
						}
					})
				})
				return
			}
			ui.Scroll(c).Height(200).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					for _, guard := range a.uninstallGuards {
						g := guard
						c.Key(g.file)
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							label := g.Exe
							if label == "" {
								label = locales.UninstallGuardUnreadable() + " · " + g.file
							}
							ui.Text(c, label).Grow(1).MinWidth(0).FontSize(12).SingleLine().Clip().Tooltip(label)
							ui.Button(c, locales.UninstallGuardRelease()).OnClick(func() { a.uninstallGuardSelected, a.uninstallGuardConfirm = g, true })
						})
					}
				})
			})
			ui.Row(c).Justify(ui.End).Children(func() { ui.Button(c, locales.DialogOK()).OnClick(func() { a.uninstallGuardOpen = false }) })
		})
	})
}
