package main

import (
	"context"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func main() {
	// A command runs without a window; the window opens with none.
	if args := os.Args[1:]; len(args) > 0 {
		if args[0] == "--apply-pending-update" {
			os.Exit(runUpdateHelper(args[1:]))
		}
		os.Exit(runCLI(args, os.Stdout, scanAll))
	}

	a := newApp()
	// One window: starting the app again brings the running one forward.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if isManagedInstall() {
		if _, err := readPendingUpdate(); err == nil {
			if err := startUpdateHelper(os.Getpid()); err == nil {
				return
			}
		}
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		if a.win == nil {
			return
		}
		a.win.Show()
		a.win.Focus()
	})
	mygo.App.WhenReady(func() {
		_ = startLogging(false)
		logger.Printf("start %s, data %s", appVersion, userDataDir())
		a.load()
		a.updateCtx, a.updateCancel = context.WithCancel(context.Background())
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:           locales.AppTitle(),
			Width:           1180,
			Height:          760,
			MinWidth:        900,
			MinHeight:       560,
			TitleBarStyle:   mygo.TitleBarHidden,
			TitleBarHeight:  36,
			BackgroundColor: "#19191a",
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(a.view),
		})
		a.win.SetWindowButtonsTheme(mygo.ThemeDark)
		// What the window was made with does not follow the language on its
		// own.
		mygo.App.OnLocaleChanged(func(string) {
			a.win.SetTitle(locales.AppTitle())
		})
		mygo.App.OnBeforeQuit(func(event *mygo.QuitEvent) {
			if a.moduleBusy {
				event.PreventDefault()
				a.manageOpen, a.manageModules = true, true
				return
			}
			if a.hubCancel != nil {
				a.hubCancel()
			}
			if a.scanCancel != nil {
				a.scanCancel()
			}
			if a.updateCancel != nil {
				a.updateCancel()
			}
		})
		a.startHubWatch()
		a.refresh()
		if isManagedInstall() {
			a.startAutomaticUpdates()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
