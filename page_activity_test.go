package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestModuleBackgroundActivityAndCancellation(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newApp()
	a.manageEditor = queueFixture()
	a.manageOpen, a.manageModules, a.moduleBusy = true, true, true
	a.moduleCurrentName, a.moduleStatus = "Android Build Support", locales.InstallModuleProgress()
	a.moduleProgressDone, a.moduleProgressTotal = 4<<20, 10<<20
	cancelled := false
	a.moduleCancel = func() { cancelled = true }
	tt := ui.NewTester(a.view, 1180, 760)
	if err := tt.Click(locales.ActionBackground()); err != nil {
		t.Fatal(err)
	}
	if a.manageOpen || a.page != pageActivity || !a.moduleBusy {
		t.Fatal("backgrounding interrupted the task")
	}
	if !tt.HasText("4.0 MB / 10.0 MB") || !tt.HasText("Android Build Support") {
		t.Fatalf("missing progress: %q", tt.Texts())
	}
	if a.moduleProgressFraction() != .4 {
		t.Fatalf("wrong fraction: %v", a.moduleProgressFraction())
	}
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("background cancellation was not sent")
	}
	a.moduleInstalling = true
	a.moduleStatus = locales.DownloadsInstalling()
	tt.Frame()
	if a.moduleProgressFraction() != -1 || tt.HasText("4.0 MB / 10.0 MB") {
		t.Fatal("installation reported stale download progress")
	}
	if err := tt.Click(locales.InstallModules()); err != nil {
		t.Fatal(err)
	}
	if !a.manageOpen || !a.moduleBusy {
		t.Fatal("could not return to the running task")
	}
}

func TestBackgroundEditorJobsAcrossEngines(t *testing.T) {
	a := newApp()
	unity := &downloadJob{Entry: downloadEntry{Engine: kindUnity, Version: "7000.0.0a16"}, Status: "downloading", Done: 10, Total: 20}
	tuanjie := &downloadJob{Entry: downloadEntry{Engine: kindTuanjie, Version: "2022.3.62t17"}, Status: "installing"}
	a.jobs = map[string]*downloadJob{jobKey(unity.Entry): unity, jobKey(tuanjie.Entry): tuanjie}
	a.page = pageActivity
	tt := ui.NewTester(a.view, 1180, 760)
	if len(a.activityJobs(true)) != 2 || !tt.HasText(channelName(kindUnity)+" · 7000.0.0a16") || !tt.HasText(channelName(kindTuanjie)+" · 2022.3.62t17") {
		t.Fatalf("jobs missing: %q", tt.Texts())
	}
}
