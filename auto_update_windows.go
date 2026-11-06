//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func managedUpdateExe() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "UnityStarterMSI", "UnityStarter.exe")
}

func isManagedInstall() bool {
	exe, err := os.Executable()
	return err == nil && strings.EqualFold(filepath.Clean(exe), filepath.Clean(managedUpdateExe()))
}

// A separate copy can wait for the running app to exit without locking the
// installed executable while Windows Installer replaces it.
func startUpdateHelper(pid int) error {
	source, err := os.Executable()
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(updatesDir(), 0o700); err != nil {
		return err
	}
	out, err := os.CreateTemp(updatesDir(), "updater-*.exe")
	if err != nil {
		return err
	}
	path := out.Name()
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(path)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(path)
		return err
	}
	if err := startDetached(path, []string{"--apply-pending-update", strconv.Itoa(pid)}, updatesDir()); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func runUpdateHelper(args []string) int {
	if len(args) != 1 {
		return 2
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil || pid <= 0 {
		return 2
	}
	pending, err := readPendingUpdate()
	if err != nil {
		return 1
	}
	if err = waitForUpdateParent(pid); err == nil {
		var rechecked pendingUpdate
		rechecked, err = readPendingUpdate()
		if err == nil && rechecked != pending {
			err = fmt.Errorf("pending update changed while waiting")
		}
	}
	if err == nil {
		msiexec := filepath.Join(os.Getenv("WINDIR"), "System32", "msiexec.exe")
		cmd := exec.Command(msiexec, "/i", updateMSIPath(pending.Version), "/qn", "/norestart")
		hideWindowCmd(cmd)
		err = cmd.Run()
	}
	if err == nil {
		var digest string
		digest, err = fileSHA256(managedUpdateExe())
		if err == nil && digest != pending.ExeSHA256 {
			err = fmt.Errorf("installed executable checksum mismatch")
		}
	}
	// Never retry a failed installation in a restart loop; a manual check can
	// explicitly retry it. Windows Installer rolls back failed upgrades.
	_ = os.Remove(pendingUpdatePath())
	if err != nil {
		recordUpdateFailure(pending.Version, err)
		_ = startUpdatedApp()
		return 1
	}
	_ = os.Remove(updateFailurePath())
	if err := startUpdatedApp(); err != nil {
		recordUpdateFailure(pending.Version, err)
		return 1
	}
	return 0
}

// The updater itself is hidden, but the restarted application must have a
// normal visible window. Do not use startDetached: it sets HideWindow.
func startUpdatedApp() error {
	exe := managedUpdateExe()
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func waitForUpdateParent(pid int) error {
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		if err == syscall.Errno(87) {
			return nil
		} // process has already exited
		return err
	}
	defer syscall.CloseHandle(h)
	result, err := syscall.WaitForSingleObject(h, 60000)
	if err != nil {
		return err
	}
	if result != syscall.WAIT_OBJECT_0 {
		return fmt.Errorf("timed out waiting for UnityStarter to exit")
	}
	return nil
}
