//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestInstallerElevationDetection(t *testing.T) {
	if !needsElevation(fmt.Errorf("wrapped: %w", syscall.Errno(740))) || needsElevation(syscall.Errno(5)) || needsElevation(nil) {
		t.Fatal("invalid elevation classification")
	}
}

func TestManualInteractiveLaunch(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_LIVE_GUI_FIXTURE")
	if exe == "" {
		t.Skip("set UNITYSTARTER_LIVE_GUI_FIXTURE to an interactive GUI executable")
	}
	if err := startDetached(exe, nil, filepath.Dir(exe)); err != nil {
		t.Fatal(err)
	}
}
