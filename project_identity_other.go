//go:build !windows

package main

import "path/filepath"

func projectIdentityPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
