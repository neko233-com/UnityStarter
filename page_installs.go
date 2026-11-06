package main

import (
	"path/filepath"
	"sort"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) installsPage(c *ui.Context) {
	t := c.Theme()
	engine := engineForPage(a.page)
	editors := a.editorsForEngine(engine)
	a.header(c, channelName(engine)+" · "+locales.InstallsTitle(), locales.InstallsCount(len(editors)), func() {
		ui.Button(c, locales.ActionRefresh()).OnClick(a.refresh).Disabled(a.scanning)
		ui.Button(c, locales.ActionAddFolder()).OnClick(a.addEditorRoot)
	})
	ui.Divider(c)

	a.installs.Key = func(i int) any { return editors[i].Exe }
	ui.List(c, &a.installs, len(editors), func(i int) {
		e := editors[i]
		ui.Row(c).Padding(14, 18).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, e.Version).FontSize(16).Bold()
					ui.Badge(c, channelName(e.Kind))
					if e.Simulated {
						ui.Badge(c, locales.SimulatedBadge())
					}
				})
				ui.Text(c, e.Exe).FontSize(12).TextColor(t.TextMuted).Clip()
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
			})
		})
	}).Grow(1).Children(func() {
		if len(editors) == 0 {
			a.emptyState(c, locales.InstallsEmptyTitle(), locales.InstallsEmptyHint())
		}
	})
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
