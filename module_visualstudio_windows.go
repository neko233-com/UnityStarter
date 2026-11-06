//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

func checkVisualStudioModules(ctx context.Context, modules []editorModule) {
	needed := false
	for _, m := range modules {
		if visualStudioModuleMajor(m) != "" {
			needed = true
			break
		}
	}
	base := os.Getenv("ProgramFiles(x86)")
	if !needed || base == "" {
		return
	}
	exe := filepath.Join(base, "Microsoft Visual Studio", "Installer", "vswhere.exe")
	query := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, exe, args...)
		hideWindowCmd(cmd)
		return cmd.Output()
	}
	args := []string{"-all", "-products", "Microsoft.VisualStudio.Product.Community", "-format", "json", "-utf8"}
	all, err := query(args...)
	if err != nil {
		return
	}
	args = append(args, "-requires")
	args = append(args, visualStudioComponents...)
	ready, err := query(args...)
	if err == nil {
		applyVisualStudioState(modules, all, ready)
	}
}
