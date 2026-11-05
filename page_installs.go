package main

import (
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) installsPage(c *ui.Context) {
	t := c.Theme()
	a.header(c, locales.InstallsTitle(), locales.InstallsCount(len(a.editors)), func() {
		ui.Button(c, locales.ActionRefresh()).OnClick(a.refresh).Disabled(a.scanning)
		ui.Button(c, locales.ActionAddFolder()).OnClick(a.addEditorRoot)
	})
	ui.Divider(c)

	a.installs.Key = func(i int) any { return a.editors[i].Exe }
	ui.List(c, &a.installs, len(a.editors), func(i int) {
		e := a.editors[i]
		ui.Row(c).Padding(12, 18).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, e.Version).FontSize(14).Bold()
					ui.Badge(c, channelName(e.Kind))
				})
				ui.Text(c, e.Exe).FontSize(11).TextColor(t.TextMuted).Clip()
			})
			ui.Row(c).Gap(6).Children(func() {
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
		if len(a.editors) == 0 {
			a.emptyState(c, locales.InstallsEmptyTitle(), locales.InstallsEmptyHint())
		}
	})
}

func (a *app) addEditorRoot() {
	a.pickFolder(locales.DialogChooseFolder(), func(dir string) {
		a.prefs.EditorRoots = dedupe(append(a.prefs.EditorRoots, dir))
		a.save()
		a.refresh()
	})
}
