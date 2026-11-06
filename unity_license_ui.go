package main

import (
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

const urlUnitySoftwareTerms = "https://unity.com/legal/editor-terms-of-service/software"

func (a *app) unityLicenseMessage() string {
	if a.licenseCheckBusy {
		switch a.licenseStage {
		case "signin":
			return locales.LicenseSigningIn()
		case "activating":
			return locales.LicenseActivating()
		default:
			return locales.LicenseChecking()
		}
	}
	switch a.licenseCheckState {
	case "":
		return locales.LicenseLaunchHint()
	case "active":
		return locales.LicenseActive()
	case "inactive":
		return locales.LicenseInactive()
	case "unavailable":
		return locales.LicenseCheckUnavailable()
	case "signin-failed":
		return locales.LicenseSignInFailed()
	case "activation-failed":
		return locales.LicenseActivationFailed()
	case "cancelled":
		return locales.LicenseCancelled()
	default:
		return locales.LicenseCheckFailed()
	}
}

func (a *app) unityLicenseControls(c *ui.Context) {
	t := c.Theme()
	panel(c, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconLicense).Size(22, 22).Shrink(0).TextColor(t.TextMuted)
			ui.Text(c, locales.LicenseUnityStatus()).FontSize(16).Bold().Grow(1)
			if a.licenseCheckState == "active" && !a.licenseCheckBusy {
				ui.Badge(c, locales.LicenseReady())
			}
		})
		ui.Text(c, a.unityLicenseMessage()).FontSize(13)
		if a.licenseCheckBusy {
			ui.Text(c, locales.LicenseBackgroundHint()).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c, locales.LicenseCheck()).Disabled(a.licenseCheckBusy).OnClick(a.checkUnityLicense)
			ui.Button(c, locales.LicenseOpenHub()).Disabled(a.licenseCheckBusy).OnClick(a.openUnityLicenseHub)
			if a.licenseCheckBusy {
				ui.Button(c, locales.LicenseStop()).OnClick(a.cancelUnityLicenseFlow)
			}
		})
		ui.Divider(c)
		ui.Text(c, locales.LicensePersonalTitle()).FontSize(14).Bold()
		ui.Text(c, locales.LicensePersonalHint()).FontSize(12).TextColor(t.TextMuted)
		ui.Checkbox(c, &a.licensePersonalConsent, locales.LicensePersonalConsent()).Disabled(a.licenseCheckBusy)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.PrimaryButton(c, locales.LicenseActivatePersonal()).Disabled(a.licenseCheckBusy || !a.licensePersonalConsent).OnClick(func() { a.startUnityLicenseFlow(true) })
			ui.Button(c, locales.LicenseTerms()).OnClick(func() { _ = mygo.Shell.OpenExternal(urlUnitySoftwareTerms) })
		})
		ui.Text(c, locales.LicenseCheckHint()).FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *app) unityLicenseDialog(c *ui.Context) {
	if !a.licenseFlowOpen {
		return
	}
	w, h := c.Size()
	// Modal owns Escape/backdrop dismissal, so reconcile it to cancellation.
	if ui.Modal(c, &a.licenseFlowOpen, func() {
		ui.Column(c).Width(min(620, w-80)).MaxHeight(h - 90).Gap(12).Children(func() {
			ui.Text(c, locales.LicenseLaunchTitle()).FontSize(20).Bold().Shrink(0)
			if a.licensePendingLabel != "" {
				ui.Text(c, a.licensePendingLabel).FontSize(12).SingleLine().Clip().Shrink(0)
			}
			ui.Scroll(c).Height(390).Shrink(1).MinHeight(80).Children(func() { a.unityLicenseControls(c) })
			ui.Row(c).Gap(8).Wrap().Justify(ui.End).Shrink(0).Children(func() {
				ui.Button(c, locales.ActionCancel()).OnClick(a.cancelUnityLicenseFlow)
				if a.licensePending != nil && !a.licenseCheckBusy && (a.licenseCheckState == "unavailable" || a.licenseCheckState == "failed") {
					ui.Button(c, locales.LicenseEditorCheck()).Tooltip(locales.LicenseEditorCheckHint()).OnClick(a.resumeUnityLaunch)
				}
			})
		})
	}).Dismissed() {
		a.cancelUnityLicenseFlow()
	}
}

func (a *app) openUnityLicenseHub() {
	for _, h := range a.hubs {
		if h.Kind != kindUnity || h.Exe == "" {
			continue
		}
		if info, err := os.Stat(h.Exe); err == nil && info.Mode().IsRegular() && strings.HasSuffix(strings.ToLower(h.Exe), ".exe") {
			if err := mygo.Shell.OpenPath(h.Exe); err != nil {
				a.alert(locales.OpenFailed(err.Error()))
			}
			return
		}
	}
	_ = mygo.Shell.OpenExternal(urlLicenseUnity)
}
