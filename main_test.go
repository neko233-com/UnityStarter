package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"unitystarter/locales"
)

func fixture() *app {
	a := newApp()
	a.projects = []projectEntry{
		{
			Path:     `D:\Code\Games\demo`,
			Name:     "demo",
			Version:  "2022.3.51f1",
			Kind:     kindUnity,
			Modified: time.Date(2026, 1, 5, 13, 7, 0, 0, time.Local),
			Editor:   `C:\Program Files\Unity\Hub\Editor\2022.3.51f1\Editor\Unity.exe`,
		},
		{
			Path:     `D:\Code\Games\cozy`,
			Name:     "cozy",
			Version:  "2022.3.62t17",
			Kind:     kindTuanjie,
			Modified: time.Date(2025, 12, 1, 9, 0, 0, 0, time.Local),
		},
	}
	a.editors = []editorInstall{
		{
			Version: "2022.3.51f1",
			Kind:    kindUnity,
			Exe:     `C:\Program Files\Unity\Hub\Editor\2022.3.51f1\Editor\Unity.exe`,
			Root:    `C:\Program Files\Unity\Hub\Editor\2022.3.51f1`,
		},
	}
	return a
}

// The view runs without a window in tests, which click as a user would.
func TestView(t *testing.T) {
	a := fixture()
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText("demo") {
		t.Fatalf("texts %q lack the project", tt.Texts())
	}
	if err := tt.Click(locales.SidebarInstalls()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.InstallsTitle()) {
		t.Errorf("texts %q after choosing installs", tt.Texts())
	}
	if err := tt.Click(locales.SidebarSettings()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.SettingsLanguage()) {
		t.Errorf("texts %q after choosing settings", tt.Texts())
	}
}

// SNAPSHOT=1 go test -run TestSnapshot draws the pages to PNG files, to look
// at them.
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT")
	if dir == "" {
		t.Skip("set SNAPSHOT to a directory to draw the pages")
	}
	a := fixture()
	for _, page := range []string{pageProjects, pageInstalls, pageSettings} {
		a.page = page
		tt := ui.NewTester(a.view, 1180, 760)
		tt.Frame()
		name := filepath.Join(dir, "page"+page[1:]+".png")
		if err := writePNG(name, tt.Image()); err != nil {
			t.Fatal(err)
		}
		t.Log(name)
	}
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
