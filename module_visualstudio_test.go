package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func visualStudioFixture(name string) editorModule {
	return editorModule{ID: "visualstudio", Name: "Microsoft Visual Studio Community " + name, Command: "Microsoft.VisualStudio.Product.Community " + strings.Join(visualStudioComponents, " "), Visible: true}
}

func TestVisualStudioWorkloadDetection(t *testing.T) {
	modules := []editorModule{visualStudioFixture("2022"), visualStudioFixture("2026"), {ID: "other", Selected: true}}
	all := []byte(`[{"instanceId":"a","productId":"Microsoft.VisualStudio.Product.Community","installationVersion":"17.14.1","isComplete":true,"isLaunchable":true},{"instanceId":"b","productId":"Microsoft.VisualStudio.Product.Community","installationVersion":"18.2.1","isComplete":true,"isLaunchable":true}]`)
	ready := []byte(`[{"instanceId":"b","productId":"Microsoft.VisualStudio.Product.Community","installationVersion":"18.2.1","isComplete":true,"isLaunchable":true}]`)
	applyVisualStudioState(modules, all, ready)
	if modules[0].Selected || modules[0].ToolStatus != "incomplete" || !modules[1].Selected || modules[1].ToolStatus != "ready" || !modules[2].Selected {
		t.Fatalf("incorrect workload detection: %+v", modules)
	}
	before := modules[0]
	applyVisualStudioState(modules, []byte("broken"), []byte("[]"))
	if modules[0] != before {
		t.Fatal("failed query changed the existing state")
	}
	modules[0].Command = "unexpected installer metadata"
	modules[0].Selected = true
	applyVisualStudioState(modules, all, []byte("[]"))
	if !modules[0].Selected {
		t.Fatal("unrecognized metadata was treated as an authoritative module")
	}
}

func TestVisualStudioMissingWorkloadIsVisible(t *testing.T) {
	a := fixture()
	m := visualStudioFixture("2022")
	m.ToolStatus = "incomplete"
	a.manageEditor.Modules = []editorModule{m}
	a.manageOpen, a.manageModules = true, true
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(m.Name) || !tt.HasText(locales.VisualStudioWorkloadMissing()) {
		t.Fatalf("missing workload not visible: %q", tt.Texts())
	}
}

func TestLiveVisualStudioModuleDetection(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_VISUAL_STUDIO") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_VISUAL_STUDIO=1 to inspect local Visual Studio")
	}
	modules := []editorModule{visualStudioFixture("2022"), visualStudioFixture("2026")}
	if root := os.Getenv("UNITYSTARTER_LIVE_VISUAL_STUDIO_ROOT"); root != "" {
		modules = readEditorModules(root)
		if len(modules) == 0 {
			t.Fatal("no Hub module metadata found")
		}
	}
	checkVisualStudioModules(context.Background(), modules)
	for _, m := range modules {
		if m.ID == "visualstudio" {
			t.Logf("%s: selected=%v status=%s", m.Name, m.Selected, m.ToolStatus)
		}
	}
}
