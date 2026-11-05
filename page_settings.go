package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func (a *app) settingsPage(c *ui.Context) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 26).Gap(26).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(24).Bold()
			a.languageSection(c)

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

func (a *app) aboutSection(c *ui.Context) {
	t := c.Theme()
	version := mygo.App.Version()
	if version == "" {
		version = "0.1.0"
	}
	ui.Column(c).Gap(6).Children(func() {
		a.sectionTitle(c, locales.SettingsAbout(), locales.SettingsAboutText())
		ui.Text(c, locales.SettingsVersion(version)).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, prefsPath()).FontSize(11).TextColor(t.TextMuted).Clip()
		ui.Row(c).Children(func() {
			if ui.Button(c, locales.ActionOpenDataDir()).Clicked() {
				if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
					_ = mygo.Shell.OpenPath(dir)
				}
			}
		})
	})
}
