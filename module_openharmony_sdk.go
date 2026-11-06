package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const openHarmonySDKModule = "openharmony-sdk"

// The exact SDK package and digest are published by OpenHarmony. Unknown
// catalog versions stay unavailable until their archive layout is validated.
func openHarmonySDKArchive(m editorModule) (string, bool) {
	if m.ID != openHarmonySDKModule || m.Parent != "openharmony" ||
		m.URL != "https://repo.huaweicloud.com/openharmony/os/6.1-Release/ohos-sdk-windows_linux-public.tar.gz" ||
		m.Destination != "{UNITY_PATH}/Editor/Data/PlaybackEngines/OpenHarmonyPlayer" ||
		m.Command != "" || m.EULA != "" || m.EULA2 != "" || m.Checksum != "" || m.Integrity != "" || m.RenameFrom != "" || m.RenameTo != "" {
		return "", false
	}
	sum, _ := hex.DecodeString("b833b75a64ee46bbd7880921abbb49b733ec5c8171b6684c9b524d57f624cee0")
	return "sha256-" + base64.StdEncoding.EncodeToString(sum), true
}

func openHarmonySDKFilesPresent(root string) bool {
	dir := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer", "SDK", "23")
	return openHarmonySDKComponentsPresent(dir)
}

func openHarmonySDKComponentsPresent(dir string) bool {
	for _, rel := range []string{"native/llvm/bin/clang.exe", "ets/build-tools/ets-loader/package.json", "toolchains/hdc.exe"} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			return false
		}
	}
	for _, component := range []string{"native", "ets", "js", "previewer", "toolchains"} {
		file := filepath.Join(dir, component, "oh-uni-package.json")
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1<<20 {
			return false
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		var metadata struct {
			APIVersion json.RawMessage `json:"apiVersion"`
			Version    string          `json:"version"`
		}
		if json.Unmarshal(data, &metadata) != nil || metadata.Version != "6.1.0.31" {
			return false
		}
		var api string
		if json.Unmarshal(metadata.APIVersion, &api) != nil {
			api = string(metadata.APIVersion)
		}
		if api != "23" {
			return false
		}
	}
	return true
}

// Extract only the Windows component zips into a temporary sibling. The
// destination is committed only after every archive and API version checks.
func installOpenHarmonySDK(root, archive, integrity string) error {
	if !strings.HasPrefix(integrity, "sha256-") {
		return fmt.Errorf("invalid SDK integrity")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	parent := root
	for _, part := range []string{"Editor", "Data", "PlaybackEngines", "OpenHarmonyPlayer"} {
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("SDK parent is missing or unsafe")
		}
	}
	dest := filepath.Join(parent, "SDK")
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("SDK destination is unsafe")
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("SDK destination already contains files")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	hash, expected, err := integrityHasher(integrity)
	if err != nil || hash == nil {
		return fmt.Errorf("invalid SDK integrity")
	}
	if _, err = io.Copy(hash, f); err != nil {
		return err
	}
	if err = verifyIntegrity(hash, expected); err != nil {
		return err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	stage, err := os.MkdirTemp(parent, ".unitystarter-sdk-")
	if err != nil {
		return err
	}
	defer func() {
		if filepath.Dir(stage) == parent && strings.HasPrefix(filepath.Base(stage), ".unitystarter-sdk-") {
			_ = os.RemoveAll(stage)
		}
	}()
	contents := filepath.Join(stage, "contents")
	if err := os.Mkdir(contents, 0o755); err != nil {
		return err
	}
	reader := tar.NewReader(gz)
	count, components := 0, 0
	seen := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		if count > 1024 || header.Size < 0 || header.Size > 4<<30 {
			return fmt.Errorf("invalid SDK archive size")
		}
		name := header.Name
		clean := path.Clean(name)
		if name == "" || strings.ContainsAny(name, `\\:`) || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("unsafe SDK archive path")
		}
		for _, part := range strings.Split(clean, "/") {
			if !safeArchiveComponent(part) {
				return fmt.Errorf("unsafe SDK archive path")
			}
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unsupported SDK archive entry")
		}
		if !strings.Contains("/"+clean, "/windows/") || !strings.HasSuffix(strings.ToLower(clean), ".zip") {
			continue
		}
		key := strings.ToLower(clean)
		if seen[key] {
			return fmt.Errorf("duplicate SDK component")
		}
		seen[key] = true
		component := filepath.Join(stage, "component.zip")
		out, err := os.OpenFile(component, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		digest := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(out, digest), reader)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil || written != header.Size {
			return fmt.Errorf("SDK component incomplete")
		}
		checksum := "sha256-" + base64.StdEncoding.EncodeToString(digest.Sum(nil))
		if err := extractVerifiedArchive(component, contents, checksum, 150000); err != nil {
			return err
		}
		if err := os.Remove(component); err != nil {
			return err
		}
		components++
	}
	if components == 0 {
		return fmt.Errorf("SDK Windows components missing")
	}
	if !openHarmonySDKComponentsPresent(contents) {
		return fmt.Errorf("SDK components are incomplete or have unexpected versions")
	}
	api := strconv.Itoa(23)
	if err := os.Rename(contents, filepath.Join(stage, api)); err != nil {
		return err
	}
	removedEmpty := false
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Remove(dest); err != nil {
			return err
		}
		removedEmpty = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, dest); err != nil {
		if removedEmpty {
			_ = os.Mkdir(dest, 0o755)
		}
		return err
	}
	return nil
}
