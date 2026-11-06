//go:build windows

package main

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// Go's EvalSymlinks does not resolve Windows directory junctions. Ask the OS
// for the final path of the opened directory, including its long name.
func projectIdentityPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(windows.Handle(f.Fd()), &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if n < uint32(len(buf)) {
			result := windows.UTF16ToString(buf[:n])
			if strings.HasPrefix(result, `\\?\UNC\`) {
				return `\\` + result[len(`\\?\UNC\`):], nil
			}
			return strings.TrimPrefix(result, `\\?\`), nil
		}
		if n > 32768 {
			return "", windows.ERROR_FILENAME_EXCED_RANGE
		}
		buf = make([]uint16, n+1)
	}
}
