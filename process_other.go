//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

func startDetached(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func hideWindowCmd(cmd *exec.Cmd) {}

func startHiddenDetached(exe string, args []string, dir string) error {
	return startDetached(exe, args, dir)
}

func configureInstallerCmd(cmd *exec.Cmd, engine, dest string) {}

func runEditorInstaller(exe, engine, dest string) error {
	return exec.Command(exe, installerArgs(engine, dest)...).Run()
}

func runEditorUninstaller(exe string) error {
	return fmt.Errorf("native editor uninstall is only supported on Windows")
}

func editorProcessRunning(exe string) (bool, error) {
	return false, fmt.Errorf("native editor uninstall is only supported on Windows")
}
