package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	m.ToolStatus, m.ToolPath, m.ToolInstanceID = "incomplete", t.TempDir(), "test-instance"
	a.manageEditor.Modules = []editorModule{m}
	a.manageOpen, a.manageModules = true, true
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(m.Name) || !tt.HasText(locales.VisualStudioModify()) {
		t.Fatalf("missing workload not visible: %q", tt.Texts())
	}
}

func TestVisualStudioWatchRequiresExactInstanceAndCanStop(t *testing.T) {
	m := visualStudioFixture("2022")
	m.ToolStatus, m.ToolPath, m.ToolInstanceID = "incomplete", t.TempDir(), "target"
	target := visualStudioInstance{InstanceID: "target", ProductID: "Microsoft.VisualStudio.Product.Community", InstallationVersion: "17.14.1", InstallationPath: m.ToolPath, IsComplete: true, IsLaunchable: true}
	other := target
	other.InstanceID, other.InstallationPath = "other", t.TempDir()
	all, _ := json.Marshal([]visualStudioInstance{target, other})
	otherReady, _ := json.Marshal([]visualStudioInstance{other})
	targetReady, _ := json.Marshal([]visualStudioInstance{target})
	if visualStudioTargetReady(m, all, otherReady) {
		t.Fatal("another instance was treated as the target")
	}
	calls := 0
	query := func(context.Context) ([]byte, []byte, error) {
		calls++
		if calls == 1 {
			return all, otherReady, nil
		}
		return all, targetReady, nil
	}
	if err := waitVisualStudioReady(context.Background(), m, time.Millisecond, query); err != nil || calls != 2 {
		t.Fatalf("watch failed: calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitVisualStudioReady(ctx, m, time.Millisecond, query); err != context.Canceled || calls != 2 {
		t.Fatalf("canceled watch queried or returned success: calls=%d err=%v", calls, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := waitVisualStudioReady(ctx, m, time.Millisecond, func(context.Context) ([]byte, []byte, error) {
		return all, otherReady, nil
	})
	if err != context.DeadlineExceeded {
		t.Fatalf("unverified instance should time out: %v", err)
	}
}

func TestVisualStudioWatchActivityCanBeDismissed(t *testing.T) {
	a := fixture()
	a.page = pageActivity
	canceled := false
	a.visualStudioTask = &visualStudioTask{Module: visualStudioFixture("2022"), Status: "waiting", Cancel: func() { canceled = true }}
	tt := ui.NewTester(a.view, 1180, 760)
	if !tt.HasText(locales.VisualStudioWaiting()) {
		t.Fatalf("watch status missing: %q", tt.Texts())
	}
	if err := tt.Click(locales.VisualStudioStopWatching()); err != nil {
		t.Fatal(err)
	}
	if !canceled || a.visualStudioTask != nil {
		t.Fatal("stop did not dismiss the monitor")
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
