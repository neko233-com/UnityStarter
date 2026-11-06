package main

import (
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

// Shared native controls keep focus, keyboard and accessibility behavior while
// giving every workspace the same restrained Hub-style hierarchy.
func pageTab(c *ui.Context, label string, selected bool, choose func()) {
	t := c.Theme()
	c.Key(label)
	b := ui.Button(c, label).Height(38).Padding(0, 10).Radius(0).
		Background(ui.Color{}).Border(0, ui.Color{}).TextColor(t.TextMuted).OnClick(choose)
	if selected {
		b.TextColor(t.Text).Bold().BorderColor(t.Accent).BorderWidth(0, 0, 2, 0)
	}
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
}

func panel(c *ui.Context, children func()) {
	t := c.Theme()
	ui.Column(c).Padding(18).Gap(12).Background(t.Surface).Border(1, t.Border).Radius(6).Children(children)
}

func tableHeading(c *ui.Context, label, sort string, active bool, reverse bool, choose func()) ui.Element {
	t := c.Theme()
	text := label
	if active {
		descending := !reverse
		if sort == "name" {
			descending = reverse
		}
		if descending {
			text += " ↓"
		} else {
			text += " ↑"
		}
	}
	c.Key(sort)
	b := ui.Button(c, text).Label(label).Tooltip(label).Height(30).Padding(0, 8).
		Background(ui.Color{}).Border(0, ui.Color{}).FontSize(12).TextColor(t.TextMuted).OnClick(choose)
	if sort != "favorite" {
		b.Justify(ui.Start)
	}
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	return b
}

func (a *app) searchEmpty(c *ui.Context, clear func()) {
	ui.Column(c).Grow(1).Center().Padding(24).Gap(12).Children(func() {
		ui.Icon(c, iconProjects).Size(30, 30).TextColor(c.Theme().TextMuted)
		ui.Text(c, locales.SearchEmpty()).FontSize(16).Bold()
		ui.Text(c, locales.SearchEmptyHint()).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Button(c, locales.SearchClear()).OnClick(clear)
	})
}
