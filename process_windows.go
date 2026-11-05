//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// startDetached starts a program without a console window of its own, and
// without waiting for it: the editor outlives this app.
func startDetached(exe string, args []string, dir string) error {
	cmd := exec.Command(exe, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// hideWindowCmd keeps a command's console window from flashing.
func hideWindowCmd(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
