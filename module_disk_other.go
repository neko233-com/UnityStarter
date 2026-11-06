//go:build !windows

package main

func moduleDiskSpace(path string) (int64, bool) { return 0, false }
