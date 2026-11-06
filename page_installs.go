package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) installsPage(c *ui.Context) {
	t := c.Theme()
	engine := engineForPage(a.page)
	editors := a.filteredEditors(engine)
	a.header(c, locales.InstallsTitle(), locales.InstallsCount(len(editors)), func() {
		ui.Button(c, locales.ActionRefresh()).OnClick(a.refresh).Disabled(a.scanning)
		ui.Button(c, locales.ActionAddFolder()).OnClick(a.addEditorRoot)
		ui.PrimaryButton(c, locales.InstallEditor()).OnClick(func() { _, _, page, _ := enginePages(engine); a.page = page })
	})
	ui.Row(c).Padding(0, 24, 10, 24).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.SettingsInstallRoot()+":").FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, a.installRoot()).Grow(1).MinWidth(0).FontSize(12).TextColor(t.TextMuted).SingleLine().Clip().Tooltip(a.installRoot())
		ui.Button(c, locales.InstallChooseLocation()).OnClick(a.chooseInstallRoot)
	})
	ui.Row(c).Padding(0, 24, 0, 24).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
		for _, tab := range []struct{ id, label string }{{"", locales.InstallAll()}, {"official", locales.InstallOfficial()}, {"preview", locales.InstallPreview()}} {
			tab := tab
			pageTab(c, tab.label, a.installFilter == tab.id, func() { a.installFilter = tab.id })
		}
		ui.Spacer(c)
		ui.SearchField(c, &a.installQuery).Placeholder(locales.InstallSearch()).Label(locales.InstallSearch()).Width(210).Margin(0, 0, 8, 0)
	})
	ui.Divider(c)

	a.installs.Key = func(i int) any { return editors[i].Exe }
	ui.List(c, &a.installs, len(editors), func(i int) {
		e := editors[i]
		ui.Column(c).Padding(8, 24).Children(func() {
			ui.Row(c).Padding(16).Gap(14).AlignItems(ui.Center).Background(t.Surface).Border(1, t.Border).Radius(6).Children(func() {
				ui.Box(c).Size(44, 44).Shrink(0).Radius(6).Background(t.Background).Center().Children(func() {
					ui.Icon(c, iconBrand).Size(26, 26).TextColor(t.TextMuted)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						version := displayVersion(e.Version, e.DisplayVersion)
						ui.Text(c, version).FontSize(15).Bold().SingleLine().Clip().Tooltip(version)
						if e.Simulated {
							ui.Badge(c, locales.SimulatedBadge())
						}
					})
					ui.Text(c, e.Exe).FontSize(12).TextColor(t.TextMuted).SingleLine().Clip().Tooltip(e.Exe)
					modules := installedModulesPreview(e.Modules, 3)
					if len(modules) > 0 {
						ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
							for _, m := range modules {
								ui.Badge(c, moduleChipName(m)).MaxWidth(120).Clip().Tooltip(m.Name)
							}
							count := len(installedModuleNames(e.Modules))
							if count > len(modules) {
								ui.Button(c, fmt.Sprintf("+%d", count-len(modules))).Height(22).Padding(0, 8).Radius(11).Label(locales.InstalledModulesShowAll(count)).Tooltip(locales.InstalledModulesShowAll(count)).OnClick(func() {
									a.installedModulesEditor, a.installedModulesOpen = e, true
								})
							}
						})
					}
				})
				ui.Row(c).Gap(6).Shrink(0).Children(func() {
					if e.Simulated {
						if ui.Button(c, locales.ActionFolder()).Clicked() {
							mygo.Shell.ShowItemInFolder(e.Exe)
						}
						if ui.Button(c, locales.ActionRemove()).Clicked() {
							if err := removeSimulatedEditor(e.Root); err != nil {
								a.alert(err.Error())
							} else {
								a.refresh()
							}
						}
						return
					}
					if ui.PrimaryButton(c, locales.InstallsLaunch()).Disabled(a.uninstallBusy && strings.EqualFold(e.Exe, a.manageEditor.Exe)).Clicked() {
						if err := launchWithEditorLock(e.Exe, nil, filepath.Dir(e.Exe), startDetached); err != nil {
							a.alert(locales.OpenFailed(err.Error()))
						}
					}
					if ui.Button(c, locales.ActionFolder()).Clicked() {
						mygo.Shell.ShowItemInFolder(e.Exe)
					}
					otherBusy := (a.moduleBusy || a.moduleLoading || a.uninstallBusy || len(a.moduleQueue) > 0 || a.androidPlan != nil) && !strings.EqualFold(a.manageEditor.Exe, e.Exe)
					ui.Button(c, locales.InstallManage()).Disabled(otherBusy).OnClick(func() { a.openEditorManagement(e) })
				})
			})
		})
	}).Grow(1).Children(func() {
		if len(editors) == 0 {
			if a.installQuery != "" || a.installFilter != "" {
				a.searchEmpty(c, func() { a.installQuery, a.installFilter = "", "" })
			} else {
				a.emptyState(c, locales.InstallsEmptyTitle(), locales.InstallsEmptyHint())
			}
		}
	})
}

// Platform modules appear first; the full list, including dependencies and
// Hub-reported entries, remains available through the overflow button.
func installedModulesPreview(modules []editorModule, limit int) []editorModule {
	var platform, other []editorModule
	for _, m := range modules {
		if !m.Selected || m.Name == "" {
			continue
		}
		if moduleOutput(m) != "" {
			platform = append(platform, m)
		} else {
			other = append(other, m)
		}
	}
	all := append(platform, other...)
	return all[:min(max(limit, 0), len(all))]
}

func moduleChipName(m editorModule) string {
	switch m.ID {
	case "android":
		return "Android"
	case "ios":
		return "iOS"
	case "windows-il2cpp":
		return "Windows"
	case "webgl":
		return "WebGL"
	case "weixinminigame":
		return "Mini Game"
	case "playableads":
		return "Playable Ads"
	case "linux-il2cpp":
		return "Linux IL2CPP"
	case "linux-mono":
		return "Linux Mono"
	case "documentation":
		return locales.ModuleGroupDocs()
	}
	name := []rune(strings.TrimSpace(m.Name))
	if len(name) > 18 {
		return string(name[:17]) + "…"
	}
	return string(name)
}

func (a *app) installedModulesDialog(c *ui.Context) {
	if !a.installedModulesOpen {
		return
	}
	e := a.installedModulesEditor
	_, height := c.Size()
	ui.Modal(c, &a.installedModulesOpen, func() {
		ui.Column(c).Width(560).Gap(12).Children(func() {
			ui.Text(c, locales.InstalledModulesTitle(len(installedModuleNames(e.Modules)))).FontSize(17).Bold()
			ui.Text(c, displayVersion(e.Version, e.DisplayVersion)).FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Scroll(c).Height(min(float32(390), max(float32(180), height-270))).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, m := range e.Modules {
						if !m.Selected || m.Name == "" {
							continue
						}
						ui.Row(c).Padding(7, 10).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, m.Name).Grow(1).FontSize(12).SingleLine().Clip().Tooltip(m.Name)
							if m.PresenceUnverified {
								ui.Text(c, locales.InstallModuleReported()).FontSize(11).TextColor(c.Theme().TextMuted)
							}
						})
					}
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.DialogOK()).OnClick(func() { a.installedModulesOpen = false })
				if !e.Simulated {
					same := strings.EqualFold(a.manageEditor.Exe, e.Exe)
					blocked := a.uninstallBusy || (same && a.uninstallStatus != "") || ((a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil) && !same)
					ui.PrimaryButton(c, locales.InstallModules()).Disabled(blocked).OnClick(func() {
						a.installedModulesOpen = false
						a.openEditorManagement(e)
						if a.manageOpen && !a.manageUninstall {
							a.manageModules = true
							if len(a.manageEditor.Modules) == 0 || a.manageEditor.ModulesCached {
								a.loadEditorModules(a.manageEditor)
							}
						}
					})
				}
			})
		})
	})
}

func (a *app) openEditorManagement(e editorInstall) {
	same := a.manageEditor.Exe != "" && strings.EqualFold(a.manageEditor.Exe, e.Exe)
	if (a.moduleBusy || a.moduleLoading || a.uninstallBusy || len(a.moduleQueue) > 0 || a.androidPlan != nil) && !same {
		return
	}
	if same && (a.uninstallBusy || a.uninstallStatus != "") {
		a.manageOpen, a.manageModules, a.manageUninstall = true, false, true
		return
	}
	if same && (a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil || (a.manageModules && a.moduleStatus != "")) {
		a.manageOpen, a.manageModules, a.manageUninstall = true, true, false
		return
	}
	a.manageEditor, a.manageOpen = e, true
	a.moduleDiskFree, a.moduleDiskKnown = 0, false
	if a.win != nil {
		win := a.win
		go func() {
			free, ok := moduleDiskSpace(e.Root)
			win.Update(func() {
				if strings.EqualFold(a.manageEditor.Exe, e.Exe) {
					a.moduleDiskFree, a.moduleDiskKnown = free, ok
				}
			})
		}()
	}
	a.manageModules, a.manageUninstall = false, false
	a.moduleStatus, a.androidPlan = "", nil
	a.moduleSelection, a.moduleQueue, a.moduleQueueTotal = nil, nil, 0
	a.moduleQueueWarnings = nil
	a.moduleSelectionRestored = false
	selection, err := a.restoreModuleSelection(e)
	if err != nil {
		a.moduleStatus = locales.ModuleSelectionReadFailed()
		a.alert(locales.ModuleSelectionReadFailed())
	} else if len(selection) > 0 {
		a.moduleSelection = selection
		a.moduleSelectionRestored = true
	}
	a.uninstallStatus = ""
}

func (a *app) uninstallActivityBanner(c *ui.Context) {
	if a.manageOpen || (!a.uninstallBusy && a.uninstallStatus == "") {
		return
	}
	ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		if a.uninstallBusy {
			ui.Spinner(c)
		}
		ui.Text(c, a.manageEditor.Version+" · "+a.uninstallStatus).Grow(1).FontSize(12).Clip()
		ui.Button(c, locales.InstallManage()).OnClick(func() { a.openEditorManagement(a.manageEditor) })
	})
}

// A dismissed modal must not strand an active module operation. Keep a
// route back to its status and cancellation on every page and engine tab.
func (a *app) moduleActivityBanner(c *ui.Context) {
	active := a.moduleBusy || a.moduleLoading || len(a.moduleQueue) > 0 || a.androidPlan != nil
	if a.manageOpen || a.page == pageActivity || (!active && (!a.manageModules || a.moduleStatus == "")) {
		return
	}
	status := a.moduleStatus
	if status == "" {
		if a.androidPlan != nil {
			status = locales.AndroidLicenseTitle()
		} else {
			status = locales.InstallModuleLoading()
		}
	}
	ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		if active && a.androidPlan == nil {
			ui.Spinner(c)
		}
		ui.Text(c, displayVersion(a.manageEditor.Version, a.manageEditor.DisplayVersion)+" · "+a.moduleCurrentName+" · "+status).Grow(1).FontSize(12).Clip()
		if a.moduleBusy {
			ui.Progress(c, a.moduleProgressFraction()).Width(130)
		}
		ui.Button(c, locales.InstallModules()).OnClick(func() { a.openEditorManagement(a.manageEditor) })
	})
}

func (a *app) filteredEditors(engine string) []editorInstall {
	var result []editorInstall
	q := strings.ToLower(strings.TrimSpace(a.installQuery))
	for _, e := range a.editorsForEngine(engine) {
		version := strings.ToLower(e.Version)
		preview := strings.ContainsAny(version, "ab")
		if a.installFilter == "preview" && !preview || a.installFilter == "official" && preview {
			continue
		}
		if !strings.Contains(version, q) && !strings.Contains(strings.ToLower(e.DisplayVersion), q) && !strings.Contains(strings.ToLower(e.Exe), q) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func (a *app) editorsForEngine(engine string) []editorInstall {
	var result []editorInstall
	for _, e := range a.editors {
		if e.Kind == engine {
			result = append(result, e)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return compareVersions(result[i].Version, result[j].Version) > 0 })
	return result
}

func (a *app) addEditorRoot() {
	a.pickFolder(locales.DialogChooseFolder(), func(dir string) {
		a.prefs.EditorRoots = dedupe(append(a.prefs.EditorRoots, dir))
		a.save()
		a.refresh()
	})
}
