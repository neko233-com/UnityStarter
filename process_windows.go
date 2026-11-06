//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
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
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: syscall.EscapeArg(cmd.Path) + " /S " + arg}
}

func needsElevation(err error) bool { return errors.Is(err, syscall.Errno(740)) }

// Windows asks for elevation only when the installer requires it. Keep its
// process handle so an elevation denial or installer failure is not success.
func runEditorInstaller(exe, engine, dest string) error {
	return runEditorInstallerWith(exe, engine, dest, runElevatedInstaller)
}

func runEditorInstallerWith(exe, engine, dest string, elevate func(string, string) error) error {
	cmd := exec.Command(exe, installerArgs(engine, dest)...)
	configureInstallerCmd(cmd, engine, dest)
	err := cmd.Run()
	if !needsElevation(err) {
		return err
	}
	return elevate(exe, dest)
}

type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	window     uintptr
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instance   uintptr
	idList     uintptr
	class      *uint16
	classKey   uintptr
	hotKey     uint32
	icon       uintptr
	process    syscall.Handle
}

func runElevatedInstaller(exe, dest string) error {
	return runElevatedCommand(exe, "/S /D="+dest, "")
}

func runEditorUninstaller(exe string) error {
	return runEditorUninstallerWith(exe, runElevatedCommand)
}

func runEditorUninstallerWith(exe string, elevate func(string, string, string) error) error {
	// Unity copies its uninstaller to a temporary location; its working
	// directory must not be inside the editor installation.
	dir := filepath.VolumeName(exe) + string(os.PathSeparator)
	cmd := exec.Command(exe, "/S")
	cmd.Dir = dir
	hideWindowCmd(cmd)
	err := cmd.Run()
	if needsElevation(err) {
		return elevate(exe, "/S", dir)
	}
	return err
}

func editorProcessRunning(exe string) (bool, error) {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(snapshot)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err = syscall.Process32First(snapshot, &entry); err != nil {
		return false, err
	}
	query := syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
	for {
		if strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), filepath.Base(exe)) {
			h, openErr := syscall.OpenProcess(0x1000, false, entry.ProcessID)
			if openErr != nil {
				// A process can exit between the snapshot and OpenProcess.
				if !errors.Is(openErr, syscall.Errno(87)) {
					return false, fmt.Errorf("cannot inspect editor process %d: %w", entry.ProcessID, openErr)
				}
			} else {
				buf := make([]uint16, 32768)
				n := uint32(len(buf))
				ok, _, callErr := query.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
				syscall.CloseHandle(h)
				if ok == 0 {
					return false, fmt.Errorf("cannot inspect editor process %d: %w", entry.ProcessID, callErr)
				}
				path := syscall.UTF16ToString(buf[:n])
				if resolved, err := filepath.EvalSymlinks(path); err == nil {
					path = resolved
				}
				if strings.EqualFold(filepath.Clean(path), filepath.Clean(exe)) {
					return true, nil
				}
			}
		}
		err = syscall.Process32Next(snapshot, &entry)
		if errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func runElevatedCommand(exe, args, dir string) error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	parameters, err := syscall.UTF16PtrFromString(args)
	if err != nil {
		return err
	}
	var directory *uint16
	if dir != "" {
		directory, err = syscall.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
	}
	info := shellExecuteInfo{size: uint32(unsafe.Sizeof(shellExecuteInfo{})), mask: 0x40 | 0x100, verb: verb, file: file, parameters: parameters, directory: directory, show: 0}
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW")
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	if result == 0 {
		if callErr == syscall.Errno(0) {
			return fmt.Errorf("could not start elevated installer")
		}
		return callErr
	}
	if info.process == 0 {
		return fmt.Errorf("installer did not return a process handle")
	}
	defer syscall.CloseHandle(info.process)
	resultCode, err := syscall.WaitForSingleObject(info.process, 0xffffffff)
	if err != nil {
		return err
	}
	if resultCode != syscall.WAIT_OBJECT_0 {
		return fmt.Errorf("installer wait failed: %d", resultCode)
	}
	var exit uint32
	if err := syscall.GetExitCodeProcess(info.process, &exit); err != nil {
		return err
	}
	if exit != 0 {
		return fmt.Errorf("installer exited with code %d", exit)
	}
	return nil
}
