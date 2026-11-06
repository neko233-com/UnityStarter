package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// Coordinate GUI and CLI operations on the same physical editor directory.
// The lock is held only while launching, but throughout an uninstall.
func acquireEditorLock(exe string) (func(), error) {
	dir, err := filepath.Abs(filepath.Dir(exe))
	if err != nil {
		return nil, err
	}
	// During a fresh install the Editor directory may not exist yet; resolve
	// the closest existing ancestor so aliases use the same lock later.
	var suffix []string
	for current := dir; ; current = filepath.Dir(current) {
		if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			dir = resolved
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		suffix = append(suffix, filepath.Base(current))
	}
	sum := sha256.Sum256([]byte(localProjectKey(dir)))
	release, err := acquireDownloadLock("editor-" + hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, fmt.Errorf("editor is busy in another UnityStarter operation: %w", err)
	}
	return release, nil
}

func launchWithEditorLock(exe string, args []string, dir string, launch func(string, []string, string) error) error {
	release, err := acquireEditorLock(exe)
	if err != nil {
		return err
	}
	defer release()
	return launch(exe, args, dir)
}
