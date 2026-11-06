package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Coordinate GUI and CLI operations on the same physical editor directory.
// The lock is held only while launching, but throughout an uninstall.
func editorLockID(exe string) (string, error) {
	dir, err := filepath.Abs(filepath.Dir(exe))
	if err != nil {
		return "", err
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
	return hex.EncodeToString(sum[:]), nil
}

func acquireEditorLockRaw(exe string) (func(), error) {
	id, err := editorLockID(exe)
	if err != nil {
		return nil, err
	}
	release, err := acquireDownloadLock("editor-" + id)
	if err != nil {
		return nil, fmt.Errorf("editor is busy in another UnityStarter operation: %w", err)
	}
	return release, nil
}

func acquireEditorLock(exe string) (func(), error) {
	release, err := acquireEditorLockRaw(exe)
	if err != nil {
		return nil, err
	}
	path, err := uninstallGuardPath(exe)
	if err == nil {
		_, err = os.Lstat(path)
	}
	if err == nil {
		release()
		return nil, errUninstallProtected
	}
	if !os.IsNotExist(err) {
		release()
		return nil, fmt.Errorf("cannot check uninstall protection: %w", err)
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
