package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	instances := []visualStudioInstance{
		{InstanceID: "a", ProductID: "Microsoft.VisualStudio.Product.Community", InstallationVersion: "17.14.1", InstallationPath: filepath.Join(t.TempDir(), "VS 2022"), IsComplete: true, IsLaunchable: true},
		{InstanceID: "b", ProductID: "Microsoft.VisualStudio.Product.Community", InstallationVersion: "18.2.1", InstallationPath: filepath.Join(t.TempDir(), "VS 2026"), IsComplete: true, IsLaunchable: true},
	}
	all, _ := json.Marshal(instances)
	ready, _ := json.Marshal(instances[1:])
	applyVisualStudioState(modules, all, ready)
	if modules[0].Selected || modules[0].ToolStatus != "incomplete" || !modules[1].Selected || modules[1].ToolStatus != "ready" || !modules[2].Selected {
		t.Fatalf("incorrect workload detection: %+v", modules)
	}
	args, err := visualStudioModifyArgs(modules[0])
	if err != nil || len(args) != 3+2*len(visualStudioComponents) || args[0] != "modify" || args[2] != instances[0].InstallationPath {
		t.Fatalf("invalid installer arguments: %q, %v", args, err)
	}
	for i, component := range visualStudioComponents {
		if args[3+i*2] != "--add" || args[4+i*2] != component {
			t.Fatalf("unexpected component arguments: %q", args)
		}
	}
	duplicate := instances[0]
	duplicate.InstanceID = "another"
	instances = append(instances, duplicate)
	ambiguous, _ := json.Marshal(instances)
	applyVisualStudioState(modules, ambiguous, ready)
	if modules[0].ToolStatus != "ambiguous" || modules[0].ToolPath != "" {
		t.Fatalf("multiple matching instances require explicit selection: %+v", modules[0])
	}
	if _, err := visualStudioModifyArgs(modules[0]); err == nil {
		t.Fatal("ambiguous instance was accepted")
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
	m.ToolStatus, m.ToolPath = "incomplete", t.TempDir()
	a.manageEditor.Modules = []editorModule{m}
	a.manageOpen, a.manageModules = true, true
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(m.Name) || !tt.HasText(locales.VisualStudioModify()) {
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
