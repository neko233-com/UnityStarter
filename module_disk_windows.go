//go:build windows

package main

import "golang.org/x/sys/windows"

func moduleDiskSpace(path string) (int64, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, nil, nil); err != nil || free > 1<<63-1 {
		return 0, false
	}
	return int64(free), true
}
