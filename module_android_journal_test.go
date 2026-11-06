package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type androidCrashFixture struct {
	Root, Base string
	Plan       androidInstallPlan
	Archives   []string
}

func TestAndroidCommitCrashChild(t *testing.T) {
	path := os.Getenv("UNITYSTARTER_ANDROID_COMMIT_CHILD")
	if path == "" {
		t.Skip("subprocess fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture androidCrashFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	cache, err := openAndroidCache(fixture.Base, fixture.Root, fixture.Plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	err = installAndroidArchivesJournal(fixture.Root, fixture.Plan, fixture.Archives, cache, func(phase string) {
		if phase == os.Getenv("UNITYSTARTER_ANDROID_COMMIT_PHASE") {
			os.Exit(91)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("commit did not reach crash hook")
}

func TestAndroidJournalRejectsEscapingStage(t *testing.T) {
	root, base := t.TempDir(), t.TempDir()
	plan, _, _ := androidCacheFixture("http://example.test")
	cache, err := openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	j := androidJournal{Version: 1, Key: cache.key, Stage: "../outside", SDKHash: strings.Repeat("a", 64), NDKHash: strings.Repeat("b", 64)}
	data, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache.dir, androidJournalName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readAndroidJournal(cache); err == nil {
		t.Fatal("escaping stage accepted")
	}
	if err := cache.clear(); err == nil {
		t.Fatal("unrecognized journal was deleted")
	}
}

func TestAndroidCommitRecoversAfterProcessCrash(t *testing.T) {
	for _, mode := range []string{"after-sdk", "after-ndk", "preserve-user-change", "preserve-conflicting-ndk"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("APPDATA", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			root, base := fakeEditorRoot(t), t.TempDir()
			plan, archives := androidArchiveFixtures(t)
			plan.Module.ID = "android-sdk-ndk-tools"
			for i, p := range archives {
				info, err := os.Stat(p)
				if err != nil {
					t.Fatal(err)
				}
				plan.Packages[i].ExpectedSize = info.Size()
			}
			fixture := androidCrashFixture{Root: root, Base: base, Plan: plan, Archives: archives}
			data, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			spec := filepath.Join(t.TempDir(), "fixture.json")
			if err := os.WriteFile(spec, data, 0o600); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestAndroidCommitCrashChild$")
			phase := "SDK"
			if mode == "after-ndk" {
				phase = "NDK"
			}
			cmd.Env = append(os.Environ(), "UNITYSTARTER_ANDROID_COMMIT_CHILD="+spec, "UNITYSTARTER_ANDROID_COMMIT_PHASE="+phase)
			hideWindowCmd(cmd)
			out, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 91 {
				t.Fatalf("child did not crash at commit: %s %v", out, err)
			}
			parent := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
			if mode != "after-ndk" && androidSDKFilesPresent(parent) {
				t.Fatal("partial commit was treated as installed")
			}
			if mode == "after-ndk" && readModuleState(root)[plan.Module.ID].Installed {
				t.Fatal("unregistered commit was marked installed")
			}
			if pending, err := androidJournalPending(base, root, plan); err != nil || !pending {
				t.Fatalf("recovery not detected: %v %v", pending, err)
			}
			if mode == "preserve-user-change" {
				if err := os.WriteFile(filepath.Join(parent, "SDK", "platform-tools", "adb.exe"), []byte("user change"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "preserve-conflicting-ndk" {
				if err := os.Mkdir(filepath.Join(parent, "NDK"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(parent, "NDK", "user.txt"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cache, err := openAndroidCache(base, root, plan)
			if err != nil {
				t.Fatal(err)
			}
			defer cache.close()
			recovered, err := recoverAndroidJournal(cache, root)
			if mode == "preserve-conflicting-ndk" {
				if err == nil || recovered {
					t.Fatalf("conflicting NDK was accepted: %v %v", recovered, err)
				}
				if got, err := os.ReadFile(filepath.Join(parent, "NDK", "user.txt")); err != nil || string(got) != "keep" {
					t.Fatalf("conflicting NDK changed: %q %v", got, err)
				}
				return
			}
			if mode == "preserve-user-change" {
				if err == nil || recovered || !strings.Contains(err.Error(), "changed") {
					t.Fatalf("modified files were accepted: %v %v", recovered, err)
				}
				if got, err := os.ReadFile(filepath.Join(parent, "SDK", "platform-tools", "adb.exe")); err != nil || string(got) != "user change" {
					t.Fatalf("user change was lost: %q %v", got, err)
				}
				if _, err := os.Stat(filepath.Join(parent, "NDK")); !os.IsNotExist(err) {
					t.Fatalf("recovery modified NDK: %v", err)
				}
				return
			}
			if err != nil || !recovered || !androidSDKFilesPresent(parent) {
				t.Fatalf("recovery failed: %v %v", recovered, err)
			}
			if err := markOwnedModuleInstalled(root, plan.Module); err != nil {
				t.Fatal(err)
			}
			if err := finalizeAndroidJournal(cache, root); err != nil {
				t.Fatal(err)
			}
			if _, pending, err := readAndroidJournal(cache); err != nil || pending {
				t.Fatalf("journal remains: %v %v", pending, err)
			}
			if err := cache.clear(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
