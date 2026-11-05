package main

import "github.com/egoist/mygo/ui"

// A neutral, dark workspace with a restrained blue accent.
var starterTheme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#292b2f")
	t.Surface = ui.Hex("#202225")
	t.SurfaceHover = ui.Hex("#373a40")
	t.SurfacePressed = ui.Hex("#454950")
	t.Border = ui.Hex("#484b52")
	t.Text = ui.Hex("#f2f3f5")
	t.TextMuted = ui.Hex("#b1b5bd")
	t.Accent = ui.Hex("#416d9f")
	t.AccentHover = ui.Hex("#527fb0")
	t.AccentPressed = ui.Hex("#31577f")
	return t
}()
