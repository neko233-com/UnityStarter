//go:build windows

package main

import (
	"errors"
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

func TestInstallerElevationFallback(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_ELEVATED_FIXTURE")
	if exe == "" {
		t.Skip("set UNITYSTARTER_ELEVATED_FIXTURE to the administrator NSIS fixture")
	}
	dest := filepath.Join(t.TempDir(), "Target with spaces")
	denied := errors.New("authorization denied")
	called := false
	err := runEditorInstallerWith(exe, kindTuanjie, dest, func(gotExe, gotDest string) error {
		called = true
		if gotExe != exe || gotDest != dest {
			t.Fatalf("wrong elevation target: %q %q", gotExe, gotDest)
		}
		return denied
	})
	if !called || !errors.Is(err, denied) {
		t.Fatalf("elevation failure was not propagated: called=%v err=%v", called, err)
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
