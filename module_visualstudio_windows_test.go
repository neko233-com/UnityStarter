//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func TestVisualStudioInstallerElevationFallback(t *testing.T) {
	base := t.TempDir()
	exe := filepath.Join(base, "Microsoft Visual Studio", "Installer", "setup.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := visualStudioFixture("2022")
	m.ToolStatus, m.ToolPath, m.ToolInstanceID = "incomplete", filepath.Join(base, "Visual Studio 2022"), "test-instance"
	want, err := visualStudioModifyArgs(m)
	if err != nil {
		t.Fatal(err)
	}
	elevated := false
	start := func(gotExe string, gotArgs []string, dir string) error {
		if gotExe != exe || dir != "" || !reflect.DeepEqual(gotArgs, want) {
			t.Fatalf("unexpected start: %s %q %s", gotExe, gotArgs, dir)
		}
		return syscall.Errno(740)
	}
	elevate := func(gotExe string, gotArgs []string, dir string) error {
		elevated = true
		if gotExe != exe || dir != "" || !reflect.DeepEqual(gotArgs, want) {
			t.Fatalf("unexpected elevation: %s %q %s", gotExe, gotArgs, dir)
		}
		return errors.New("elevation declined")
	}
	if err := startVisualStudioModifierWith(m, base, start, elevate); err == nil || err.Error() != "elevation declined" || !elevated {
		t.Fatalf("elevation failure not propagated: %v, %v", err, elevated)
	}
}
