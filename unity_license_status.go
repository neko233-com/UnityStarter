package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

var errUnityLicenseCLIUnavailable = errors.New("Unity CLI is unavailable")

// Only the status is retained: license records and account or machine IDs
// returned by the official CLI are neither displayed nor written to logs.
func parseUnityLicenseStatus(output []byte) (string, error) {
	var result struct {
		Success bool   `json:"success"`
		Command string `json:"command"`
		Data    struct {
			Active *bool `json:"active"`
		} `json:"data"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(output, &result); err != nil || result.Command != "license status" || result.Data.Active == nil {
		return "", fmt.Errorf("invalid Unity license status response")
	}
	if result.Success && *result.Data.Active && len(result.Errors) == 0 {
		return "active", nil
	}
	if !result.Success && !*result.Data.Active && len(result.Errors) == 1 && result.Errors[0].Code == "LICENSE_NOT_ACTIVE" {
		return "inactive", nil
	}
	return "", fmt.Errorf("Unity license status could not be verified")
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("Unity CLI response too large")
	}
	return b.Buffer.Write(p)
}

type unityLicenseRunner func(context.Context, ...string) ([]byte, error)

func runUnityLicenseCommand(ctx context.Context, args ...string) ([]byte, error) {
	if runtime.GOOS != "windows" || os.Getenv("LOCALAPPDATA") == "" {
		return nil, errUnityLicenseCLIUnavailable
	}
	// Unity Hub installs the standalone CLI here. Do not search PATH for
	// "unity": it could resolve to the Editor executable instead.
	exe := filepath.Join(os.Getenv("LOCALAPPDATA"), "Unity", "bin", "unity.exe")
	if info, err := os.Stat(exe); err != nil || !info.Mode().IsRegular() {
		return nil, errUnityLicenseCLIUnavailable
	}
	args = append(append([]string(nil), args...), "--format", "json", "--no-banner", "--no-log-proxy")
	if len(args) < 2 || args[0] != "auth" || args[1] != "login" {
		args = append(args, "--non-interactive")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	hideWindowCmd(cmd)
	var output boundedOutput
	cmd.Stdout = &output
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return output.Bytes(), err
}

func unityLicenseStatus(ctx context.Context) (string, error) {
	return unityLicenseStatusWith(ctx, runUnityLicenseCommand)
}

func unityLicenseStatusWith(ctx context.Context, run unityLicenseRunner) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := run(ctx, "license", "status")
	if errors.Is(err, errUnityLicenseCLIUnavailable) || ctx.Err() != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return "", err
	}
	// The CLI reports an inactive license with a nonzero exit code and a
	// structured LICENSE_NOT_ACTIVE response. Other failures remain unknown.
	state, parseErr := parseUnityLicenseStatus(output)
	if parseErr != nil || (err != nil && state != "inactive") {
		return "", fmt.Errorf("Unity license status could not be verified")
	}
	return state, nil
}

func (a *app) checkUnityLicense() {
	a.startUnityLicenseFlow(false)
}

// No CLI invocation on startup: checks are triggered by an explicit check,
// activation, or Editor launch. Unity CLI may send its own usage telemetry.
func (a *app) unityLicenseBanner(c *ui.Context) {
	if a.licenseCheckState != "inactive" || a.page == pageLicense {
		return
	}
	engine := a.activeEngine
	switch a.page {
	case pageProjects, pageFavorites, pageInstalls, pageDownloads, pageTuanjieProjects, pageTuanjieFavorites, pageTuanjieInstalls, pageTuanjieDownloads:
		engine = engineForPage(a.page)
	}
	if engine != kindUnity {
		return
	}
	ui.Row(c).Padding(8, 22).Gap(10).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, locales.LicenseInactiveBanner()).FontSize(12).Grow(1).Clip()
		ui.Button(c, locales.LicenseView()).OnClick(func() { a.page = pageLicense })
	})
}
