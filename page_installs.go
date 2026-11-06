package main

import (
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
	ui.Row(c).Padding(0, 22, 14, 22).Gap(8).AlignItems(ui.Center).Children(func() {
		for _, tab := range []struct{ id, label string }{{"", locales.InstallAll()}, {"official", locales.InstallOfficial()}, {"preview", locales.InstallPreview()}} {
			tab := tab
			if a.installFilter == tab.id {
				ui.PrimaryButton(c, tab.label).OnClick(func() { a.installFilter = tab.id })
			} else {
				ui.Button(c, tab.label).OnClick(func() { a.installFilter = tab.id })
			}
		}
		ui.Spacer(c)
		ui.SearchField(c, &a.installQuery).Placeholder(locales.InstallSearch()).Width(210)
	})
	ui.Divider(c)

	a.installs.Key = func(i int) any { return editors[i].Exe }
	ui.List(c, &a.installs, len(editors), func(i int) {
		e := editors[i]
		ui.Row(c).Padding(18, 22).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBrand).Size(28, 28).TextColor(t.TextMuted)
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, displayVersion(e.Version, e.DisplayVersion)).FontSize(16).Bold()
					ui.Badge(c, channelName(e.Kind))
					if e.Simulated {
						ui.Badge(c, locales.SimulatedBadge())
					}
				})
				ui.Text(c, e.Exe).FontSize(12).TextColor(t.TextMuted).Clip()
				modules := installedModuleNames(e.Modules)
				if len(modules) > 0 {
					ui.Text(c, strings.Join(modules, " · ")).FontSize(11).TextColor(t.TextMuted).Clip()
				}
			})
			ui.Row(c).Gap(6).Children(func() {
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
				if ui.PrimaryButton(c, locales.InstallsLaunch()).Clicked() {
					if err := startDetached(e.Exe, nil, filepath.Dir(e.Exe)); err != nil {
						a.alert(locales.OpenFailed(err.Error()))
					}
				}
				if ui.Button(c, locales.ActionFolder()).Clicked() {
					mygo.Shell.ShowItemInFolder(e.Exe)
				}
				ui.Button(c, locales.InstallManage()).Disabled(a.moduleBusy).OnClick(func() {
					a.manageEditor = e
					a.manageOpen = true
					a.manageModules = false
					a.manageUninstall = false
					a.moduleStatus = ""
				})
			})
		})
	}).Grow(1).Children(func() {
		if len(editors) == 0 {
			a.emptyState(c, locales.InstallsEmptyTitle(), locales.InstallsEmptyHint())
		}
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
