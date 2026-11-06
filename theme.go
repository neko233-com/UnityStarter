package main

import "github.com/egoist/mygo/ui"

// A neutral, dark workspace with a restrained blue accent.
var starterTheme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#19191a")
	t.Surface = ui.Hex("#242424")
	t.SurfaceHover = ui.Hex("#363638")
	t.SurfacePressed = ui.Hex("#444446")
	t.Border = ui.Hex("#414143")
	t.Text = ui.Hex("#f2f3f5")
	t.TextMuted = ui.Hex("#b1b5bd")
	t.Accent = ui.Hex("#3c76b9")
	t.AccentHover = ui.Hex("#508acb")
	t.AccentPressed = ui.Hex("#305f98")
	return t
}()
