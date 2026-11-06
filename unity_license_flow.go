package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"unitystarter/locales"
)

// The CLI owns credentials and license files. Only fixed outcome codes cross
// into app state; raw errors, account records and OAuth URLs never do.
type unityLicenseResponse struct {
	Success bool   `json:"success"`
	Command string `json:"command"`
	Data    struct {
		LoggedIn *bool `json:"loggedIn"`
	} `json:"data"`
	Errors []struct {
		Code string `json:"code"`
	} `json:"errors"`
}

func parseUnityLicenseResponse(output []byte, command string) (unityLicenseResponse, bool) {
	var result unityLicenseResponse
	err := json.Unmarshal(output, &result)
	return result, err == nil && result.Command == command
}

func activateUnityPersonal(ctx context.Context, consent bool, run unityLicenseRunner, stage func(string)) string {
	if !consent {
		return "consent-required"
	}
	stage("checking")
	state, err := unityLicenseStatusWith(ctx, run)
	if err != nil {
		return unityLicenseFailure(ctx, err)
	}
	if state == "active" {
		return state
	}
	output, runErr := run(ctx, "auth", "status")
	auth, valid := parseUnityLicenseResponse(output, "auth status")
	if !valid || auth.Data.LoggedIn == nil {
		return unityLicenseFailure(ctx, runErr)
	}
	if !*auth.Data.LoggedIn {
		if auth.Success || len(auth.Errors) != 1 || auth.Errors[0].Code != "NOT_SIGNED_IN" {
			return "signin-failed"
		}
		stage("signin")
		output, runErr = run(ctx, "auth", "login")
		auth, valid = parseUnityLicenseResponse(output, "auth login")
		if runErr != nil || !valid || !auth.Success || len(auth.Errors) != 0 {
			if ctx.Err() != nil {
				return "cancelled"
			}
			return "signin-failed"
		}
	} else if runErr != nil || !auth.Success || len(auth.Errors) != 0 {
		return "signin-failed"
	}
	stage("activating")
	activateCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	output, runErr = run(activateCtx, "license", "activate", "--personal", "--accept-eula")
	activation, valid := parseUnityLicenseResponse(output, "license activate")
	if runErr != nil || !valid || !activation.Success || len(activation.Errors) != 0 {
		if ctx.Err() != nil {
			return "cancelled"
		}
		return "activation-failed"
	}
	stage("checking")
	state, err = unityLicenseStatusWith(ctx, run)
	if err != nil {
		return unityLicenseFailure(ctx, err)
	}
	return state // Activation success alone is never sufficient to resume launch.
}

func unityLicenseFailure(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "cancelled"
	}
	if errors.Is(err, errUnityLicenseCLIUnavailable) {
		return "unavailable"
	}
	return "failed"
}

func (a *app) startUnityLicenseFlow(activate bool) {
	if a.licenseCheckBusy || (activate && !a.licensePersonalConsent) {
		return
	}
	run := a.licenseRunner
	if run == nil {
		run = runUnityLicenseCommand
	}
	parent := a.updateCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Minute)
	a.licenseSequence++
	sequence := a.licenseSequence
	a.licenseCancel, a.licenseCheckBusy, a.licenseStage = cancel, true, "checking"
	consent := a.licensePersonalConsent
	finish := func(state string) {
		if sequence != a.licenseSequence {
			return
		}
		a.licenseCheckBusy, a.licenseCancel, a.licenseStage = false, nil, ""
		a.licenseCheckState = state
		if state == "active" {
			a.resumeUnityLaunch()
		}
	}
	// Synchronous injection lets headless tests exercise the exact launch gate.
	// Production always has a window and runs CLI I/O off the UI thread.
	if a.win == nil {
		defer cancel()
		if a.licenseRunner == nil {
			finish("unavailable")
			return
		}
		if activate {
			finish(activateUnityPersonal(ctx, consent, run, func(stage string) { a.licenseStage = stage }))
		} else {
			state, err := unityLicenseStatusWith(ctx, run)
			if err != nil {
				state = unityLicenseFailure(ctx, err)
			}
			finish(state)
		}
		return
	}
	win := a.win
	go func() {
		defer cancel()
		var state string
		if activate {
			state = activateUnityPersonal(ctx, consent, run, func(stage string) {
				win.Update(func() {
					if sequence == a.licenseSequence {
						a.licenseStage = stage
					}
				})
			})
		} else {
			var err error
			state, err = unityLicenseStatusWith(ctx, run)
			if err != nil {
				state = unityLicenseFailure(ctx, err)
			}
		}
		if parent.Err() == nil {
			win.Update(func() { finish(state) })
		}
	}()
}

func (a *app) cancelUnityLicenseFlow() {
	if a.licenseCancel != nil {
		a.licenseCancel()
	}
	a.licenseSequence++ // Reject late completion before a new job can start.
	a.licenseCancel, a.licensePending = nil, nil
	a.licenseCheckBusy, a.licenseFlowOpen = false, false
	a.licenseStage, a.licensePendingLabel = "", ""
	a.licenseCheckState = "cancelled"
}

func (a *app) withUnityLicense(e editorInstall, ready func()) {
	if e.Kind != kindUnity {
		ready()
		return
	}
	if a.licensePending != nil || a.licenseCheckBusy {
		a.licenseFlowOpen = true
		return
	}
	a.licensePending, a.licensePendingLabel, a.licenseFlowOpen = ready, displayVersion(e.Version, e.DisplayVersion), true
	a.checkUnityLicense()
}

func (a *app) resumeUnityLaunch() {
	ready := a.licensePending
	a.licensePending, a.licensePendingLabel = nil, ""
	if ready != nil {
		a.licenseFlowOpen = false
		ready()
	}
}

func (a *app) launchInstalledEditor(e editorInstall) {
	a.launchInstalledEditorChecked(e, false)
}

func (a *app) launchInstalledEditorChecked(e editorInstall, licensed bool) {
	current, ok := a.editorInstall(e.Exe)
	if !ok || current.Kind != e.Kind || current.Simulated || !strings.EqualFold(current.Version, e.Version) {
		a.alert(locales.ProjectEditorChanged())
		return
	}
	if a.uninstallBusy && strings.EqualFold(a.manageEditor.Exe, e.Exe) {
		a.alert(locales.InstallUninstallBusy())
		return
	}
	if !licensed && e.Kind == kindUnity {
		a.withUnityLicense(e, func() { a.launchInstalledEditorChecked(e, true) })
		return
	}
	if err := launchVersionCheckedEditor(e.Exe, e.Version, nil, filepath.Dir(e.Exe), a.launchEditor); err != nil {
		a.alert(locales.OpenFailed(err.Error()))
	}
}
