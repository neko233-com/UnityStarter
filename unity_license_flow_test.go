package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

const activeUnityLicenseJSON = `{"success":true,"command":"license status","data":{"active":true},"errors":[]}`
const inactiveUnityLicenseJSON = `{"success":false,"command":"license status","data":{"active":false},"errors":[{"code":"LICENSE_NOT_ACTIVE"}]}`

func activeUnityLicenseRunner(context.Context, ...string) ([]byte, error) {
	return []byte(activeUnityLicenseJSON), nil
}

func TestUnityPersonalActivationSequence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		signedIn   bool
		loginOK    bool
		activateOK bool
		verified   bool
		want       string
	}{
		{"browser sign-in", false, true, true, true, "active"},
		{"reuse sign-in", true, true, true, true, "active"},
		{"login denied", false, false, true, true, "signin-failed"},
		{"activation denied", true, true, false, true, "activation-failed"},
		{"activation not verified", true, true, true, false, "inactive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls, stages []string
			statusCalls := 0
			run := func(ctx context.Context, args ...string) ([]byte, error) {
				cmd := strings.Join(args, " ")
				calls = append(calls, cmd)
				switch cmd {
				case "license status":
					statusCalls++
					if statusCalls > 1 && tc.verified {
						return []byte(activeUnityLicenseJSON), nil
					}
					return []byte(inactiveUnityLicenseJSON), errors.New("exit 1")
				case "auth status":
					if tc.signedIn {
						return []byte(`{"success":true,"command":"auth status","data":{"loggedIn":true}}`), nil
					}
					return []byte(`{"success":false,"command":"auth status","data":{"loggedIn":false},"errors":[{"code":"NOT_SIGNED_IN"}]}`), errors.New("exit 1")
				case "auth login":
					if tc.loginOK {
						return []byte(`{"success":true,"command":"auth login"}`), nil
					}
					return []byte(`{"success":false,"command":"auth login","errors":[{"code":"AUTH_LOGIN_FAILED","message":"private"}]}`), errors.New("private")
				case "license activate --personal --accept-eula":
					if tc.activateOK {
						return []byte(`{"success":true,"command":"license activate"}`), nil
					}
					return []byte(`{"success":false,"command":"license activate","errors":[{"code":"DENIED","message":"private"}]}`), errors.New("private")
				default:
					t.Fatalf("unexpected command: %s", cmd)
					return nil, nil
				}
			}
			got := activateUnityPersonal(context.Background(), true, run, func(stage string) { stages = append(stages, stage) })
			if got != tc.want || strings.Contains(got, "private") {
				t.Fatalf("outcome=%q calls=%q", got, calls)
			}
			if slices.Contains(calls, "auth login") == tc.signedIn {
				t.Fatalf("existing authentication was not reused: %q", calls)
			}
			if tc.want == "active" && (statusCalls != 2 || stages[len(stages)-1] != "checking") {
				t.Fatalf("missing post-activation verification: %q %q", calls, stages)
			}
		})
	}
}

func TestUnityActivationConsentAndExistingLicense(t *testing.T) {
	calls := 0
	run := func(context.Context, ...string) ([]byte, error) { calls++; return []byte(activeUnityLicenseJSON), nil }
	if got := activateUnityPersonal(context.Background(), false, run, func(string) {}); got != "consent-required" || calls != 0 {
		t.Fatalf("activation without consent: %q calls=%d", got, calls)
	}
	if got := activateUnityPersonal(context.Background(), true, run, func(string) {}); got != "active" || calls != 1 {
		t.Fatalf("existing license should be preserved: %q calls=%d", got, calls)
	}
}

func TestUnityLaunchGateDoesNotCreatePartialProjects(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	a := fixture()
	a.newDir, a.newName, a.newEditor, a.newEngine = t.TempDir(), "Game", "editor", kindUnity
	e := editorInstall{Exe: "editor", Kind: kindUnity, Version: "6000.0.2f1"}
	a.editors = []editorInstall{e}
	template := templateFixture(t, kindUnity, "com.unity.template.urp-blank", "14.0.0")
	a.newTemplates, a.newTemplateID, a.newTemplateSource = []projectTemplate{template}, template.key(), templateEditorKey(e)
	launches := 0
	a.launchEditor = func(string, []string, string) error { launches++; return nil }
	a.licenseRunner = func(context.Context, ...string) ([]byte, error) {
		return []byte(inactiveUnityLicenseJSON), errors.New("exit 1")
	}
	a.createProject()
	if launches != 0 || !a.licenseFlowOpen || a.licensePending == nil || len(pendingProjects()) != 0 {
		t.Fatalf("inactive license passed creation gate: launches=%d", launches)
	}
	if _, err := os.Stat(filepath.Join(a.newDir, "Game")); !os.IsNotExist(err) {
		t.Fatalf("inactive license created a project directory: %v", err)
	}
	a.licenseRunner = activeUnityLicenseRunner
	a.checkUnityLicense()
	if launches != 1 || a.licenseFlowOpen || a.licensePending != nil || len(pendingProjects()) != 1 {
		t.Fatalf("verified license did not resume exactly once: launches=%d", launches)
	}
	a.checkUnityLicense()
	if launches != 1 {
		t.Fatal("rechecking repeated project creation")
	}
}

func TestUnityLaunchCancellationAndEngineIsolation(t *testing.T) {
	a := fixture()
	a.licenseRunner = func(context.Context, ...string) ([]byte, error) {
		return []byte(inactiveUnityLicenseJSON), errors.New("exit 1")
	}
	launches := 0
	a.withUnityLicense(editorInstall{Kind: kindUnity}, func() { launches++ })
	a.cancelUnityLicenseFlow()
	a.licenseRunner = activeUnityLicenseRunner
	a.checkUnityLicense()
	if launches != 0 || a.licensePending != nil || a.licenseFlowOpen {
		t.Fatal("cancelled launch was resumed by a later check")
	}
	a.licenseRunner = func(context.Context, ...string) ([]byte, error) {
		t.Fatal("Tuanjie invoked Unity licensing")
		return nil, nil
	}
	a.withUnityLicense(editorInstall{Kind: kindTuanjie}, func() { launches++ })
	if launches != 1 {
		t.Fatal("Tuanjie launch was blocked by Unity licensing")
	}
}

func TestUnityPendingLaunchRechecksProjectAndEditor(t *testing.T) {
	for _, action := range []string{"project", "editor"} {
		t.Run(action, func(t *testing.T) {
			a := fixture()
			e := editorInstall{Kind: kindUnity, Version: "6000.0.2f1", Exe: "chosen"}
			a.editors = []editorInstall{e}
			path := filepath.Join(t.TempDir(), "Game")
			writeProject(t, path, e.Version)
			p := projectEntry{Path: path, Version: e.Version, Editor: e.Exe, Kind: e.Kind}
			a.licenseRunner = func(context.Context, ...string) ([]byte, error) {
				return []byte(inactiveUnityLicenseJSON), errors.New("exit 1")
			}
			launches := 0
			a.launchEditor = func(string, []string, string) error { launches++; return nil }
			if action == "project" {
				a.openProject(p)
				writeProject(t, path, "7000.0.0a16")
			} else {
				a.launchInstalledEditor(e)
				a.editors[0].Version = "7000.0.0a16"
			}
			if a.licensePending == nil {
				t.Fatal("launch was not retained")
			}
			a.licenseRunner = activeUnityLicenseRunner
			a.checkUnityLicense()
			if launches != 0 || !a.alertOpen || a.licensePending != nil {
				t.Fatal("launch used stale project or editor after licensing")
			}
		})
	}
}

func TestUnityLicenseDialogEscapeCancelsPendingLaunch(t *testing.T) {
	a := fixture()
	a.licenseFlowOpen = true
	a.licensePending = func() { t.Fatal("dismissed launch resumed") }
	ctx, cancel := context.WithCancel(context.Background())
	a.licenseCancel = cancel
	tt := ui.NewTester(a.view, 900, 560)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.licensePending != nil || a.licenseFlowOpen || ctx.Err() == nil {
		t.Fatal("Escape did not cancel both the CLI operation and pending launch")
	}
}

func TestRealInactiveUnityEditorIsNotLaunched(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_LIVE_UNITY_EDITOR")
	if exe == "" || os.Getenv("UNITYSTARTER_EXPECT_LICENSE_STATE") != "inactive" {
		t.Skip("set UNITYSTARTER_LIVE_UNITY_EDITOR and UNITYSTARTER_EXPECT_LICENSE_STATE=inactive for the real launch gate")
	}
	version := editorExecutableVersion(exe)
	if version == "" {
		t.Fatal("real Editor version could not be read")
	}
	a := newApp()
	a.licenseRunner = runUnityLicenseCommand
	e := editorInstall{Exe: exe, Version: version, Kind: kindUnity}
	a.editors = []editorInstall{e}
	a.launchEditor = func(string, []string, string) error { t.Fatal("inactive real Editor was launched"); return nil }
	a.launchInstalledEditor(e)
	if a.licenseCheckState != "inactive" || a.licensePending == nil || !a.licenseFlowOpen {
		t.Fatalf("real Editor was not held for licensing: state=%q", a.licenseCheckState)
	}
	a.cancelUnityLicenseFlow()
}

func TestUnityLicenseDialogAtMinimumWindow(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans"} {
		t.Run(lang, func(t *testing.T) {
			i18n.SetLocaleFunc(func() string { return lang })
			defer i18n.SetLocaleFunc(func() string { return "en" })
			a := fixture()
			a.licenseFlowOpen, a.licenseCheckState = true, "inactive"
			a.licensePending = func() { t.Fatal("inactive license resumed") }
			a.licensePendingLabel = "7000.0.0a16"
			tt := ui.NewTester(a.view, 900, 560)
			if tt.HasText(locales.LicenseEditorCheck()) {
				t.Fatal("inactive license offered the unknown-status fallback")
			}
			for _, label := range []string{locales.LicenseLaunchTitle(), locales.LicenseCheck(), locales.LicenseActivatePersonal(), locales.ActionCancel()} {
				r, ok := tt.Find(label)
				if !ok || r.X < 0 || r.Y < 36 || r.X+r.W > 900 || r.Y+r.H > 560 {
					t.Fatalf("%q outside window: %+v", label, r)
				}
			}
			if dir := os.Getenv("LICENSE_FLOW_SNAPSHOTS"); dir != "" {
				if err := writePNG(filepath.Join(dir, fmt.Sprintf("license-%s.png", lang)), tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
			if err := tt.Click(locales.ActionCancel()); err != nil {
				t.Fatal(err)
			}
			if a.licenseFlowOpen || a.licensePending != nil {
				t.Fatal("Cancel retained pending launch")
			}
			tt.Frame()
		})
	}
}
