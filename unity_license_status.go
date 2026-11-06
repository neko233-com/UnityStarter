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
		return 0, fmt.Errorf("license status response too large")
	}
	return b.Buffer.Write(p)
}

func unityLicenseStatus(ctx context.Context) (string, error) {
	if runtime.GOOS != "windows" || os.Getenv("LOCALAPPDATA") == "" {
		return "", errUnityLicenseCLIUnavailable
	}
	// Unity Hub installs the standalone CLI here. Do not search PATH for
	// "unity": it could resolve to the Editor executable instead.
	exe := filepath.Join(os.Getenv("LOCALAPPDATA"), "Unity", "bin", "unity.exe")
	if info, err := os.Stat(exe); err != nil || !info.Mode().IsRegular() {
		return "", errUnityLicenseCLIUnavailable
	}
	cmd := exec.CommandContext(ctx, exe, "license", "status", "--format", "json", "--non-interactive")
	hideWindowCmd(cmd)
	var output boundedOutput
	cmd.Stdout = &output
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	// The CLI reports an inactive license with a nonzero exit code and a
	// structured LICENSE_NOT_ACTIVE response. Other failures remain unknown.
	state, parseErr := parseUnityLicenseStatus(output.Bytes())
	if parseErr != nil || (err != nil && state != "inactive") {
		return "", fmt.Errorf("Unity license status could not be verified")
	}
	return state, nil
}

func (a *app) checkUnityLicense() {
	if a.win == nil || a.licenseCheckBusy {
		return
	}
	a.licenseCheckBusy = true
	win := a.win
	parent := a.updateCtx
	if parent == nil {
		parent = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		state, err := unityLicenseStatus(ctx)
		if err != nil {
			state = "failed"
			if errors.Is(err, errUnityLicenseCLIUnavailable) {
				state = "unavailable"
			}
		}
		if parent.Err() != nil {
			return
		}
		win.Update(func() { a.licenseCheckState, a.licenseCheckBusy = state, false })
	}()
}
