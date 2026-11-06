//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManualInteractiveLaunch(t *testing.T) {
	exe := os.Getenv("UNITYSTARTER_LIVE_GUI_FIXTURE")
	if exe == "" {
		t.Skip("set UNITYSTARTER_LIVE_GUI_FIXTURE to an interactive GUI executable")
	}
	if err := startDetached(exe, nil, filepath.Dir(exe)); err != nil {
		t.Fatal(err)
	}
}
