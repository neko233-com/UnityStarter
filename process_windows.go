//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// startDetached starts an interactive program that outlives this app.
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

func startHiddenDetached(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// hideWindowCmd keeps a command's console window from flashing.
func hideWindowCmd(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

// NSIS treats /D= as the final raw command line argument and requires the
// Unity destination to remain unquoted, even when it contains spaces.
func configureInstallerCmd(cmd *exec.Cmd, engine, dest string) {
	arg := "/D=" + dest
	if engine == kindTuanjie {
		arg = `/D="` + dest + `"`
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: syscall.EscapeArg(cmd.Path) + " /S " + arg}
}
