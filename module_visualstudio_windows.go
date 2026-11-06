//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
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

func startVisualStudioModifier(module editorModule) error {
	return startVisualStudioModifierWith(module, os.Getenv("ProgramFiles(x86)"), startDetached, startVisualStudioElevated)
}

func startVisualStudioModifierWith(module editorModule, base string, start, elevate func(string, []string, string) error) error {
	args, err := visualStudioModifyArgs(module)
	if err != nil {
		return err
	}
	if base == "" {
		return fmt.Errorf("Visual Studio Installer directory is unavailable")
	}
	exe := filepath.Join(base, "Microsoft Visual Studio", "Installer", "setup.exe")
	if info, err := os.Stat(exe); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Visual Studio Installer is unavailable")
	}
	err = start(exe, args, "")
	if needsElevation(err) {
		return elevate(exe, args, "")
	}
	return err
}

// This is an interactive Microsoft installer: Windows may request elevation
// and the user completes any installer confirmation in its own window.
func startVisualStudioElevated(exe string, args []string, _ string) error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = syscall.EscapeArg(arg)
	}
	parameters, err := syscall.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return err
	}
	info := shellExecuteInfo{size: uint32(unsafe.Sizeof(shellExecuteInfo{})), mask: 0x40 | 0x100, verb: verb, file: file, parameters: parameters, show: 1}
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW")
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	if result == 0 {
		return callErr
	}
	if info.process != 0 {
		_ = syscall.CloseHandle(info.process)
	}
	return nil
}
