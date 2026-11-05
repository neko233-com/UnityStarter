//go:build !windows

package main

import "os/exec"

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
