package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		a.load()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppTitle(),
			Width:     1180,
			Height:    760,
			MinWidth:  900,
			MinHeight: 560,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(a.view),
		})
		// What the window was made with does not follow the language on its
		// own.
		mygo.App.OnLocaleChanged(func(string) {
			a.win.SetTitle(locales.AppTitle())
		})
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
			if a.hubCancel != nil {
				a.hubCancel()
			}
		})
		a.startHubWatch()
		a.refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
