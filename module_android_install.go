package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// downloadAndroidPackages downloads at most three archives concurrently; each
// archive may itself use the range downloader. The caller owns the temp dir.
func downloadAndroidPackages(ctx context.Context, plan androidInstallPlan, dir string, progress func(int64, int64)) ([]string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	paths := make([]string, len(plan.Packages))
	jobs := make(chan int, len(paths))
	for i := range paths {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := make([]int64, len(paths))
	var total int64
	for _, p := range plan.Packages {
		total += p.ExpectedSize
	}
	for worker := 0; worker < 3 && worker < len(paths); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				p := plan.Packages[i]
				paths[i] = filepath.Join(dir, fmt.Sprintf("%03d.zip", i))
				err := downloadFileVerifiedSize(ctx, p.Module.URL, paths[i], p.Integrity, p.ExpectedSize, func(n, _ int64) {
					mu.Lock()
					defer mu.Unlock()
					done[i] = n
					var sum int64
					for _, value := range done {
						sum += value
					}
					progress(sum, total)
				})
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}

func androidPlayerDir(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	for _, part := range []string{"Editor", "Data", "PlaybackEngines", "AndroidPlayer"} {
		root = filepath.Join(root, part)
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Android platform is missing or unsafe")
		}
	}
	return root, nil
}

func emptyAndroidDestination(parent, name string) (bool, error) {
	p := filepath.Join(parent, name)
	info, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("Android %s destination is unsafe", name)
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return false, err
	}
	if len(entries) != 0 {
		return false, fmt.Errorf("Android %s already contains files", name)
	}
	return true, nil
}

func removeAndroidStage(parent, stage, prefix string) {
	rel, err := filepath.Rel(parent, stage)
	if err == nil && filepath.Dir(rel) == "." && strings.HasPrefix(rel, prefix) {
		_ = os.RemoveAll(stage)
	}
}

func renameAndroidArchive(stage, from, to string) error {
	if from == "" && to == "" {
		return nil
	}
	fromPath := filepath.Join(stage, filepath.FromSlash(from))
	toPath := filepath.Join(stage, filepath.FromSlash(to))
	if _, err := os.Lstat(toPath); err == nil {
		// The NDK archive contains NDK/android-ndk-<version>, which becomes
		// the NDK directory itself; move it aside before removing its parent.
		if filepath.Dir(fromPath) != toPath {
			return fmt.Errorf("Android archive rename destination exists")
		}
		tmp, err := os.MkdirTemp(stage, ".android-rename-")
		if err != nil {
			return err
		}
		if err = os.Remove(tmp); err != nil {
			return err
		}
		if err = os.Rename(fromPath, tmp); err != nil {
			return err
		}
		if err = os.Remove(toPath); err != nil {
			return err
		}
		return os.Rename(tmp, toPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(fromPath, toPath)
}

func androidSDKFilesPresent(player string) bool {
	for _, pattern := range []string{
		"SDK/platform-tools/adb.exe", "SDK/build-tools/*/aapt.exe", "SDK/platforms/*/android.jar", "SDK/cmake/*/bin/cmake.exe", "SDK/cmdline-tools/*/bin/sdkmanager.bat", "NDK/source.properties",
	} {
		matches, err := filepath.Glob(filepath.Join(player, filepath.FromSlash(pattern)))
		if err != nil {
			return false
		}
		found := false
		for _, match := range matches {
			if info, err := os.Stat(match); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// installAndroidArchives extracts all packages into one sibling directory,
// then publishes SDK and NDK only after every digest and expected file passes.
func installAndroidArchives(root string, plan androidInstallPlan, archives []string) error {
	if len(plan.Packages) == 0 || len(archives) != len(plan.Packages) || strings.TrimSpace(plan.License) == "" {
		return fmt.Errorf("incomplete Android archive set")
	}
	const prefix = "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/"
	for _, pkg := range plan.Packages {
		if _, err := androidRelativePath(prefix + pkg.Destination); err != nil {
			return err
		}
		if pkg.RenameFrom != "" || pkg.RenameTo != "" {
			if _, err := androidRelativePath(prefix + pkg.RenameFrom); err != nil {
				return err
			}
			if _, err := androidRelativePath(prefix + pkg.RenameTo); err != nil {
				return err
			}
			if strings.Split(pkg.RenameFrom, "/")[0] != strings.Split(pkg.RenameTo, "/")[0] {
				return fmt.Errorf("invalid Android archive rename")
			}
		}
	}
	parent, err := androidPlayerDir(root)
	if err != nil {
		return err
	}
	existed := map[string]bool{}
	for _, name := range []string{"SDK", "NDK"} {
		existed[name], err = emptyAndroidDestination(parent, name)
		if err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp(parent, ".unitystarter-android-")
	if err != nil {
		return err
	}
	defer removeAndroidStage(parent, stage, ".unitystarter-android-")
	for i, pkg := range plan.Packages {
		dest := filepath.Join(stage, filepath.FromSlash(pkg.Destination))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		if err := extractVerifiedArchive(archives[i], dest, pkg.Integrity, 150000); err != nil {
			return fmt.Errorf("%s: %w", pkg.Module.ID, err)
		}
		if err := renameAndroidArchive(stage, pkg.RenameFrom, pkg.RenameTo); err != nil {
			return fmt.Errorf("%s: %w", pkg.Module.ID, err)
		}
	}
	if !androidSDKFilesPresent(stage) {
		return fmt.Errorf("Android SDK or NDK files are incomplete")
	}
	// Android's SDK manager records an accepted license as the SHA-1 of the
	// exact repository text. The user explicitly accepted it before this job.
	licenseDir := filepath.Join(stage, "SDK", "licenses")
	if err := os.MkdirAll(licenseDir, 0o755); err != nil {
		return err
	}
	licensePath := filepath.Join(licenseDir, "android-sdk-license")
	if info, err := os.Lstat(licensePath); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("Android license path is unsafe")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(licensePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	digest := sha1.Sum([]byte(plan.License))
	_, writeErr := file.WriteString("\n" + hex.EncodeToString(digest[:]) + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	var removed []string
	for _, name := range []string{"SDK", "NDK"} {
		if existed[name] {
			if err := os.Remove(filepath.Join(parent, name)); err != nil {
				for _, previous := range removed {
					_ = os.Mkdir(filepath.Join(parent, previous), 0o755)
				}
				return err
			}
			removed = append(removed, name)
		}
	}
	if err := os.Rename(filepath.Join(stage, "SDK"), filepath.Join(parent, "SDK")); err != nil {
		for _, name := range []string{"SDK", "NDK"} {
			if existed[name] {
				_ = os.Mkdir(filepath.Join(parent, name), 0o755)
			}
		}
		return err
	}
	if err := os.Rename(filepath.Join(stage, "NDK"), filepath.Join(parent, "NDK")); err != nil {
		if rollbackErr := os.Rename(filepath.Join(parent, "SDK"), filepath.Join(stage, "SDK")); rollbackErr != nil {
			return fmt.Errorf("Android NDK commit failed: %v; SDK rollback failed: %w", err, rollbackErr)
		}
		for _, name := range []string{"SDK", "NDK"} {
			if existed[name] {
				_ = os.Mkdir(filepath.Join(parent, name), 0o755)
			}
		}
		return err
	}
	return nil
}
