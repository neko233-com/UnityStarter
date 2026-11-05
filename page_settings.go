package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) settingsPage(c *ui.Context) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 26).Gap(26).MaxWidth(780).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(24).Bold()
			a.languageSection(c)
			a.hubSection(c)

			a.rootsSection(c, locales.SettingsProjectRoots(), locales.SettingsProjectRootsHint(), a.prefs.ProjectRoots,
				func(next []string) {
					a.prefs.ProjectRoots = next
					a.save()
					a.refresh()
				},
				func() {
					a.pickFolder(locales.DialogChooseFolder(), func(dir string) {
						a.prefs.ProjectRoots = dedupe(append(a.prefs.ProjectRoots, dir))
						a.save()
						a.refresh()
					})
				})

			a.rootsSection(c, locales.SettingsEditorRoots(), locales.SettingsEditorRootsHint(), a.prefs.EditorRoots,
				func(next []string) {
					a.prefs.EditorRoots = next
					a.save()
					a.refresh()
				},
				func() {
					a.pickFolder(locales.DialogChooseFolder(), func(dir string) {
						a.prefs.EditorRoots = dedupe(append(a.prefs.EditorRoots, dir))
						a.save()
						a.refresh()
					})
				})

			a.installRootSection(c)
			a.demoSection(c)

			a.rootsSection(c, locales.SettingsPinnedProjects(), locales.SettingsPinnedProjectsHint(), a.prefs.PinnedProjects,
				func(next []string) {
					a.prefs.PinnedProjects = next
					a.save()
					a.refresh()
				},
				a.addProject)

			a.aboutSection(c)
		})
	})
}

func (a *app) sectionTitle(c *ui.Context, title, hint string) {
	t := c.Theme()
	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, title).FontSize(15).Bold()
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
	})
}

func (a *app) languageSection(c *ui.Context) {
	labels := []string{locales.SettingsLanguageSystem(), "English", "简体中文"}
	tags := []string{"", "en", "zh-Hans"}
	index := 0
	switch {
	case a.prefs.Locale == "":
		index = 0
	case strings.HasPrefix(a.prefs.Locale, "zh"):
		index = 2
	default:
		index = 1
	}
	chosen := labels[index]

	ui.Column(c).Gap(8).Children(func() {
		a.sectionTitle(c, locales.SettingsLanguage(), locales.SettingsLanguageHint())
		ui.Select(c, &chosen, labels).Width(220)
	})

	for i, label := range labels {
		if label == chosen && i != index {
			a.prefs.Locale = tags[i]
			a.save()
			mygo.App.SetLocale(a.prefs.Locale)
		}
	}
}

// rootsSection lists folders, with a button to add one and one to remove
// each.
func (a *app) rootsSection(c *ui.Context, title, hint string, roots []string, set func([]string), add func()) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		a.sectionTitle(c, title, hint)
		if len(roots) == 0 {
			ui.Text(c, locales.SettingsRootsEmpty()).FontSize(12).TextColor(t.TextMuted)
		}
		for i, root := range roots {
			i, root := i, root
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, root).FontSize(12).Grow(1).Clip()
				if ui.Button(c, locales.ActionRemove()).Clicked() {
					next := append([]string{}, roots[:i]...)
					next = append(next, roots[i+1:]...)
					set(next)
				}
			})
		}
		ui.Row(c).Children(func() {
			ui.Button(c, locales.ActionAddFolder()).OnClick(add).Width(140)
		})
	})
}

// installRootSection is where downloaded editors go.
func (a *app) installRootSection(c *ui.Context) {
	t := c.Theme()
	root := a.installRoot()
	ui.Column(c).Gap(8).Children(func() {
		a.sectionTitle(c, locales.SettingsInstallRoot(), locales.SettingsInstallRootHint())
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, root).FontSize(12).TextColor(t.TextMuted).Grow(1).Clip()
			if ui.Button(c, locales.ActionFolder()).Clicked() {
				_ = mygo.Shell.OpenPath(root)
			}
			if ui.Button(c, locales.ActionChoose()).Clicked() {
				a.pickFolder(locales.DialogChooseFolder(), func(dir string) {
					a.prefs.InstallRoot = dir
					a.save()
					a.refresh()
				})
			}
		})
	})
}

// demoSection turns installs into stand-ins, for trying the flow.
func (a *app) demoSection(c *ui.Context) {
	demo := a.prefs.DemoMode
	ui.Column(c).Gap(8).Children(func() {
		a.sectionTitle(c, locales.SettingsDemo(), locales.SettingsDemoHint())
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Switch(c, &demo)
			ui.Text(c, locales.SettingsDemoSwitch()).FontSize(12)
		})
	})
	if demo != a.prefs.DemoMode {
		a.prefs.DemoMode = demo
		a.save()
	}
}

// hubSection takes over the hubs' projects and favorites, and says which of
// them this app can write to.
func (a *app) hubSection(c *ui.Context) {
	t := c.Theme()
	sync := !a.prefs.NoHubSync
	ui.Column(c).Gap(8).Children(func() {
		a.sectionTitle(c, locales.SettingsHubs(), locales.SettingsHubsHint())
		if len(a.hubs) == 0 {
			ui.Text(c, locales.SettingsHubsNone()).FontSize(12).TextColor(t.TextMuted)
		}
		for _, h := range a.hubs {
			h := h
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, hubName(h.Kind)).FontSize(13).Bold()
				ui.Badge(c, hubMode(h))
				if h.Version != "" {
					ui.Text(c, h.Version).FontSize(11).TextColor(t.TextMuted)
				}
				ui.Text(c, h.Dir).FontSize(11).TextColor(t.TextMuted).Grow(1).Clip()
				if ui.Button(c, locales.ActionOpenConfig()).Clicked() {
					_ = mygo.Shell.OpenPath(h.Dir)
				}
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Switch(c, &sync)
			ui.Text(c, locales.SettingsHubSync()).FontSize(12)
		})
		ui.Text(c, locales.SettingsHubSyncHint()).FontSize(11).TextColor(t.TextMuted)
	})
	if sync == a.prefs.NoHubSync {
		a.prefs.NoHubSync = !sync
		a.save()
	}
}

func hubName(kind string) string {
	if kind == kindTuanjie {
		return locales.HubTuanjie()
	}
	return locales.HubUnity()
}

func hubMode(h hub) string {
	if h.writable() {
		return locales.HubModeBoth()
	}
	return locales.HubModeImport()
}

// hubModeCode names the mode for scripts, whatever the app's language.
func hubModeCode(h hub) string {
	if h.writable() {
		return "two-way"
	}
	return "import-only"
}

func (a *app) aboutSection(c *ui.Context) {
	t := c.Theme()
	version := mygo.App.Version()
	if version == "" {
		version = appVersion
	}
	ui.Column(c).Gap(6).Children(func() {
		a.sectionTitle(c, locales.SettingsAbout(), locales.SettingsAboutText())
		ui.Text(c, locales.SettingsVersion(version)).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, prefsPath()).FontSize(11).TextColor(t.TextMuted).Clip()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, locales.ActionOpenDataDir()).Clicked() {
				if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
					_ = mygo.Shell.OpenPath(dir)
				}
			}
			if ui.Button(c, locales.ActionOpenLogs()).Clicked() {
				_ = mygo.Shell.OpenPath(logPath())
			}
			if ui.Button(c, locales.ActionCheckUpdate()).Clicked() {
				a.checkUpdates()
			}
		})
	})
}
