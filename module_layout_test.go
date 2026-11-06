package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestModuleLayoutGroupsAndDependencies(t *testing.T) {
	modules := []editorModule{
		{ID: "node", Parent: "platform", Visible: true, Category: "Platforms"},
		{ID: "documentation", Visible: true, Category: "Documentation"},
		{ID: "platform", Visible: true, Category: "Platforms"},
		{ID: "visualstudio", Visible: true, Category: "Dev tools"},
		{ID: "language-ja", Visible: true, Category: "Language packs (Preview)"},
		{ID: "hidden", Visible: false},
	}
	groups := moduleLayout(modules)
	if len(groups[0].Rows) != 1 || groups[0].Rows[0].Module.ID != "visualstudio" || len(groups[1].Rows) != 2 || groups[1].Rows[0].Module.ID != "platform" || groups[1].Rows[1].Module.ID != "node" || groups[1].Rows[1].Depth != 1 || len(groups[2].Rows) != 1 || len(groups[3].Rows) != 1 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	modules[0].DownloadSize, modules[2].DownloadSize = 12, 30
	if size, known := selectedModuleDownloadSize(modules, map[string]bool{"node": true, "platform": true}); size != 42 || !known {
		t.Fatalf("size: %d %v", size, known)
	}
	modules[0].DownloadSize = 0
	if size, known := selectedModuleDownloadSize(modules, map[string]bool{"node": true, "platform": true}); size != 30 || known {
		t.Fatalf("unknown size: %d %v", size, known)
	}
}

func TestModuleDialogGroupsAndQueueSelection(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, width := range []int{900, 1180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			e := queueFixture()
			for i := range e.Modules {
				e.Modules[i].Category = "Platforms"
				e.Modules[i].DownloadSize = int64(i+1) * 1024 * 1024
			}
			a := newApp()
			a.manageEditor, a.manageOpen, a.manageModules = e, true, true
			tt := ui.NewTester(a.view, width, 760)
			if !tt.HasText(locales.ModuleGroupPlatforms()) || !tt.HasText(locales.ModuleDownloadSize()) || !tt.HasText(locales.ModuleDiskSize()) {
				t.Fatalf("group headers missing: %q", tt.Texts())
			}
			if err := tt.Click("Android Build Support"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("OpenJDK"); err != nil {
				t.Fatal(err)
			}
			if got, err := planModuleQueue(e, a.moduleSelection); err != nil || !reflect.DeepEqual(got, []string{"android", "android-open-jdk-17.0.18+8"}) {
				t.Fatalf("queue: %v %v", got, err)
			}
			if path := os.Getenv("UNITYSTARTER_MODULE_LAYOUT_SNAPSHOT"); path != "" && width == 1180 {
				if err := writePNG(path, tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
			tt.Frame()
		})
	}
}
