package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type androidCache struct {
	base, dir, key string
	plan           androidInstallPlan
	release        func()
}

func androidCacheIdentity(root string, plan androidInstallPlan) (string, string, error) {
	if strings.TrimSpace(plan.License) == "" {
		return "", "", fmt.Errorf("missing Android license")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	root = strings.ToLower(filepath.Clean(root))
	type packageIdentity struct {
		ID, URL, Integrity, Destination, RenameFrom, RenameTo string
		Size                                                  int64
	}
	manifest := struct {
		Version                   int
		Root, Module, LicenseHash string
		Packages                  []packageIdentity
	}{Version: 1, Root: root, Module: plan.Module.ID, LicenseHash: downloadIdentity(plan.License)}
	for _, p := range plan.Packages {
		if p.ExpectedSize <= 0 {
			return "", "", fmt.Errorf("invalid Android cache package size")
		}
		h, _, err := integrityHasher(p.Integrity)
		if err != nil || h == nil {
			return "", "", fmt.Errorf("invalid Android cache integrity")
		}
		manifest.Packages = append(manifest.Packages, packageIdentity{p.Module.ID, p.Module.URL, p.Integrity, p.Destination, p.RenameFrom, p.RenameTo, p.ExpectedSize})
	}
	if len(manifest.Packages) == 0 {
		return "", "", fmt.Errorf("empty Android package plan")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), downloadIdentity("android-module:" + root), nil
}

func readAndroidCacheMarker(dir string) ([]byte, error) {
	path := filepath.Join(dir, "manifest")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 128 {
		return nil, fmt.Errorf("Android cache marker is unsafe")
	}
	return os.ReadFile(path)
}

func androidCacheLocation(base, root string, plan androidInstallPlan) (string, string, string, error) {
	key, lock, err := androidCacheIdentity(root, plan)
	if err != nil {
		return "", "", "", err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", "", "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(base); resolveErr == nil {
		base = resolved
	} else if !os.IsNotExist(resolveErr) {
		return "", "", "", resolveErr
	}
	return base, filepath.Join(base, "plan-"+key), lock, nil
}

func openAndroidCache(base, root string, plan androidInstallPlan) (*androidCache, error) {
	key, lock, err := androidCacheIdentity(root, plan)
	if err != nil {
		return nil, err
	}
	release, err := acquireDownloadLock(lock)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*androidCache, error) { release(); return nil, err }
	if err := os.MkdirAll(base, 0o700); err != nil {
		return fail(err)
	}
	base, dir, _, err := androidCacheLocation(base, root, plan)
	if err != nil {
		return fail(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return fail(err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fail(fmt.Errorf("Android cache directory is unsafe"))
	}
	marker := filepath.Join(dir, "manifest")
	data, readErr := readAndroidCacheMarker(dir)
	if os.IsNotExist(readErr) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			return fail(fmt.Errorf("Android cache ownership cannot be verified"))
		}
		if err := writeFileAtomic(marker, []byte(key+"\n")); err != nil {
			return fail(err)
		}
	} else if readErr != nil || string(data) != key+"\n" {
		return fail(fmt.Errorf("Android cache ownership cannot be verified"))
	}
	return &androidCache{base: base, dir: dir, key: key, plan: plan, release: release}, nil
}

func (c *androidCache) close() {
	if c.release != nil {
		c.release()
		c.release = nil
	}
}

func finishAndroidCache(c *androidCache, root string, plan androidInstallPlan, installed bool, contextErr error) (int64, error) {
	if c == nil {
		return 0, nil
	}
	defer c.close()
	if !installed && errors.Is(contextErr, context.Canceled) {
		_, pending, err := readAndroidJournal(c)
		if err != nil {
			return 0, err
		}
		if pending {
			return androidCachedBytes(c.base, root, plan), nil
		}
	}
	if installed || errors.Is(contextErr, context.Canceled) {
		return 0, c.clear()
	}
	return androidCachedBytes(c.base, root, plan), nil
}

// Remove only exact files belonging to this plan. Unknown files are preserved.
func (c *androidCache) clear() error {
	rel, err := filepath.Rel(c.base, c.dir)
	if err != nil || filepath.Dir(rel) != "." || rel != "plan-"+c.key {
		return fmt.Errorf("invalid Android cache location")
	}
	info, err := os.Lstat(c.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Android cache directory is unsafe")
	}
	data, err := readAndroidCacheMarker(c.dir)
	if err != nil || string(data) != c.key+"\n" {
		return fmt.Errorf("Android cache ownership cannot be verified")
	}
	allowed := map[string]bool{"manifest": true}
	for i := range c.plan.Packages {
		archive := filepath.Join(c.dir, fmt.Sprintf("%03d.zip", i))
		part, state, _, err := resumePaths(archive)
		if err != nil {
			return err
		}
		allowed[filepath.Base(archive)], allowed[filepath.Base(part)], allowed[filepath.Base(state)] = true, true, true
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] || entry.IsDir() {
			return fmt.Errorf("Android cache contains unowned files")
		}
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.dir, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(c.dir)
}

// Cached counts are estimates; each archive is verified again before reuse.
func androidCachedBytes(base, root string, plan androidInstallPlan) int64 {
	_, dir, _, err := androidCacheLocation(base, root, plan)
	if err != nil {
		return 0
	}
	key, _, err := androidCacheIdentity(root, plan)
	if err != nil {
		return 0
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0
	}
	data, err := readAndroidCacheMarker(dir)
	if err != nil || string(data) != key+"\n" {
		return 0
	}
	var total int64
	for i, p := range plan.Packages {
		archive := filepath.Join(dir, fmt.Sprintf("%03d.zip", i))
		if info, err := os.Lstat(archive); err == nil && info.Mode().IsRegular() && info.Size() == p.ExpectedSize {
			total += info.Size()
			continue
		}
		part, statePath, _, err := resumePaths(archive)
		if err != nil {
			continue
		}
		meta, err := os.Lstat(statePath)
		if err != nil || !meta.Mode().IsRegular() || meta.Size() > 64<<10 {
			continue
		}
		data, err := os.ReadFile(statePath)
		if err != nil {
			continue
		}
		var state resumeState
		if json.Unmarshal(data, &state) != nil || state.Validator == "" || !validResumeState(state, p.Module.URL, p.Integrity, state.Validator, p.ExpectedSize) {
			continue
		}
		info, err := os.Lstat(part)
		if err != nil || !info.Mode().IsRegular() || info.Size() != p.ExpectedSize {
			continue
		}
		for _, n := range state.Completed {
			total += n
		}
	}
	return total
}
