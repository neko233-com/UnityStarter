package main

import (
	"archive/zip"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var officialArchiveDigest = regexp.MustCompile(`_([0-9a-f]{64})\.zip$`)

// The vendors publish the SHA-256 as part of these exact OpenJDK URLs. Only
// known destinations and dependencies are admitted; other archives need their
// own installation and license handling.
func openJDKArchive(m editorModule) (integrity, player string, ok bool) {
	if !moduleIDPattern.MatchString(m.ID) || m.Command != "" || m.EULA != "" || m.EULA2 != "" || m.Checksum != "" {
		return "", "", false
	}
	switch {
	case m.Parent == "android" && (m.ID == "android-open-jdk" || strings.HasPrefix(m.ID, "android-open-jdk-")):
		player = "AndroidPlayer"
	case m.Parent == "hmiandroid" && m.ID == "hmiandroid-open-jdk":
		player = "HMIAndroidPlayer"
	case m.Parent == "openharmony" && m.ID == "openharmony-open-jdk":
		player = "OpenHarmonyPlayer"
	default:
		return "", "", false
	}
	if m.Destination != "{UNITY_PATH}/Editor/Data/PlaybackEngines/"+player+"/OpenJDK" {
		return "", "", false
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host != u.Hostname() {
		return "", "", false
	}
	if u.Hostname() != "download.unity3d.com" && u.Hostname() != "download.tuanjie.cn" {
		return "", "", false
	}
	match := officialArchiveDigest.FindStringSubmatch(u.Path)
	if len(match) != 2 {
		return "", "", false
	}
	sum, err := hex.DecodeString(match[1])
	if err != nil || len(sum) != 32 {
		return "", "", false
	}
	return "sha256-" + base64.StdEncoding.EncodeToString(sum), player, true
}

func parentModuleSelected(modules []editorModule, parent string) bool {
	for _, module := range modules {
		if module.ID == parent && module.Selected {
			return true
		}
	}
	return false
}

// Unknown Hub module formats remain visible, but cannot be claimed as
// physically checked until there is a reliable file layout for them.
func moduleFilesVerifiable(m editorModule) bool {
	if moduleOutput(m) != "" {
		return true
	}
	if _, _, ok := editorLanguageModule(m); ok {
		return true
	}
	if _, _, ok := tuanjieDocumentationArchive(m); ok {
		return true
	}
	if _, ok := unityDocumentationArchive(m); ok {
		return true
	}
	if androidSDKMetadata(m) || legacyAndroidSDKMetadata(m) {
		return true
	}
	_, _, ok := openJDKArchive(m)
	return ok
}

func moduleFilesPresent(root string, m editorModule) bool {
	variants, player := modulePlayerVariants(m.ID)
	if len(variants) > 0 {
		base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", filepath.Dir(filepath.Dir(moduleOutput(m))), "Variations")
		if modulePlayerVariantsPresent(base, variants, player) {
			return true
		}
		// Unity 7 replaced Mono with CoreCLR in its server packages.
		if m.ID == "linux-server" || m.ID == "windows-server" {
			coreclr := make([]string, len(variants))
			for i, variant := range variants {
				coreclr[i] = strings.Replace(variant, "_mono", "_coreclr", 1)
			}
			return modulePlayerVariantsPresent(base, coreclr, player)
		}
		if m.ID == "mac-server" {
			coreclr, coreclrPlayer := modulePlayerVariants("mac-server-coreclr")
			return modulePlayerVariantsPresent(base, coreclr, coreclrPlayer)
		}
		return false
	}
	if m.ID == "appletv" || m.ID == "visionos" {
		base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", moduleOutput(m))
		extension := "UnityEditor.AppleTV.Extensions.dll"
		if m.ID == "visionos" {
			extension = "UnityEditor.VisionOS.Extensions.dll"
		}
		info, err := os.Stat(filepath.Join(base, extension))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
		entries, err := os.ReadDir(filepath.Join(base, "Variations", "il2cpp", "Releasearm64_managed"))
		if err == nil && len(entries) > 0 {
			return true
		}
		// Unity 7 places runtime files in the Trampoline frameworks instead.
		managed, err := os.ReadDir(filepath.Join(base, "Variations", "il2cpp", "Managed"))
		if err != nil || len(managed) == 0 {
			return false
		}
		framework := "UnityRuntime-tvOS"
		if m.ID == "visionos" {
			framework = "UnityRuntime-il2cpp"
		}
		runtime, err := os.ReadDir(filepath.Join(base, "Trampoline", "Frameworks", framework))
		return err == nil && len(runtime) > 0
	}
	if m.ID == "universal-windows-platform" {
		base := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "MetroSupport")
		for _, rel := range []string{"UnityEditor.UWP.Extensions.dll", "Players/UAP/il2cpp/x64/debug/UnityPlayer.dll", "Players/UAP/il2cpp/x64/release/UnityPlayer.dll", "Players/UAP/il2cpp/x64/master/UnityPlayer.dll"} {
			info, err := os.Stat(filepath.Join(base, filepath.FromSlash(rel)))
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return false
			}
		}
		return true
	}
	if language, _, ok := editorLanguageModule(m); ok {
		file := filepath.Join(root, "Editor", "Data", "Localization", language+".po")
		info, err := os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxLanguageBytes {
			return false
		}
		data, err := os.ReadFile(file)
		return err == nil && validLanguagePO(data, language)
	}
	if _, _, ok := tuanjieDocumentationArchive(m); ok {
		return documentationFilesPresent(filepath.Join(root, "Editor", "Data", "Documentation"))
	}
	if _, ok := unityDocumentationArchive(m); ok {
		return documentationFilesPresentIn(filepath.Join(root, "Editor", "Data", "Documentation"), "en")
	}
	if m.ID == "android-sdk-ndk-tools" && m.Parent == "android" && m.EULA == androidLicenseURL {
		return androidSDKFilesPresentForPlan(filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer"), legacyAndroidSDKMetadata(m))
	}
	if _, player, ok := openJDKArchive(m); ok {
		info, err := os.Stat(filepath.Join(root, "Editor", "Data", "PlaybackEngines", player, "OpenJDK", "bin", "java.exe"))
		return err == nil && info.Mode().IsRegular() && info.Size() > 0
	}
	output := moduleOutput(m)
	if output == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "Editor", "Data", "PlaybackEngines", output))
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(root, "Editor", "Data", "PlaybackEngines", output))
	return err == nil && len(entries) > 0
}

func modulePlayerVariantsPresent(base string, variants []string, player string) bool {
	for _, variant := range variants {
		info, err := os.Stat(filepath.Join(base, variant, player))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func modulePlayerVariants(id string) ([]string, string) {
	switch id {
	case "linux-il2cpp":
		return []string{"linux64_player_development_il2cpp", "linux64_player_nondevelopment_il2cpp"}, "UnityPlayer.so"
	case "linux-mono":
		return []string{"linux64_player_development_mono", "linux64_player_nondevelopment_mono"}, "UnityPlayer.so"
	case "linux-coreclr":
		return []string{"linux64_player_development_coreclr", "linux64_player_nondevelopment_coreclr"}, "UnityPlayer.so"
	case "linux-server":
		return []string{"linux64_server_development_mono", "linux64_server_nondevelopment_mono", "linux64_server_development_il2cpp", "linux64_server_nondevelopment_il2cpp"}, "UnityPlayer.so"
	case "windows-server":
		return []string{"win64_server_development_mono", "win64_server_nondevelopment_mono", "win64_server_development_il2cpp", "win64_server_nondevelopment_il2cpp"}, "UnityPlayer.dll"
	case "mac-coreclr":
		return []string{"macos_arm64_player_development_coreclr", "macos_arm64_player_nondevelopment_coreclr"}, filepath.Join("UnityPlayer.app", "Contents", "MacOS", "UnityPlayer")
	case "mac-server-coreclr":
		return []string{"macos_arm64_server_development_coreclr", "macos_arm64_server_nondevelopment_coreclr"}, filepath.Join("UnityPlayer", "UnityPlayer")
	case "mac-mono":
		return []string{"macos_arm64_player_development_mono", "macos_arm64_player_nondevelopment_mono", "macos_x64_player_development_mono", "macos_x64_player_nondevelopment_mono"}, filepath.Join("UnityPlayer.app", "Contents", "MacOS", "UnityPlayer")
	case "mac-server":
		return []string{"macos_arm64_server_development_mono", "macos_arm64_server_nondevelopment_mono", "macos_x64_server_development_mono", "macos_x64_server_nondevelopment_mono"}, filepath.Join("UnityPlayer", "UnityPlayer")
	default:
		return nil, ""
	}
}

// installOpenJDK extracts into an isolated sibling and commits only once the
// archive and expected Java executable have been checked. Existing non-empty
// installations are never overwritten.
func installOpenJDK(root, archive, player, integrity string) error {
	if player != "AndroidPlayer" && player != "HMIAndroidPlayer" && player != "OpenHarmonyPlayer" {
		return fmt.Errorf("unsupported module destination")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	parent := resolvedRoot
	for _, part := range []string{"Editor", "Data", "PlaybackEngines", player} {
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("module parent is missing or unsafe")
		}
	}
	dest := filepath.Join(parent, "OpenJDK")
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("OpenJDK destination is not a directory")
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("OpenJDK destination already contains files")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".unitystarter-jdk-")
	if err != nil {
		return err
	}
	defer removeJDKStage(parent, stage)
	if err := extractJDKArchive(archive, stage, integrity); err != nil {
		return err
	}
	java := filepath.Join(stage, "bin", "java.exe")
	if info, err := os.Stat(java); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("OpenJDK archive does not contain bin/java.exe")
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

func removeJDKStage(parent, stage string) {
	rel, err := filepath.Rel(parent, stage)
	if err != nil || filepath.Dir(rel) != "." || !strings.HasPrefix(rel, ".unitystarter-jdk-") {
		return
	}
	_ = os.RemoveAll(stage)
}

func extractJDKArchive(archive, stage, integrity string) error {
	if !strings.HasPrefix(integrity, "sha256-") {
		return fmt.Errorf("missing or invalid OpenJDK integrity")
	}
	return extractVerifiedArchive(archive, stage, integrity, 20000)
}

func extractVerifiedArchive(archive, stage, integrity string, maxEntries int) error {
	return extractVerifiedArchiveWithMD5(archive, stage, integrity, maxEntries, false)
}

func extractVerifiedArchiveWithMD5(archive, stage, integrity string, maxEntries int, allowMD5 bool) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	h, expected, err := integrityHasher(integrity)
	if err != nil || h == nil || !(strings.HasPrefix(integrity, "sha1-") || strings.HasPrefix(integrity, "sha256-") || strings.HasPrefix(integrity, "sha384-") || (allowMD5 && strings.HasPrefix(integrity, "md5-"))) {
		return fmt.Errorf("missing or invalid archive integrity")
	}
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if err := verifyIntegrity(h, expected); err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		return err
	}
	if maxEntries <= 0 || len(z.File) > maxEntries {
		return fmt.Errorf("archive has too many entries")
	}
	seen := map[string]bool{}
	var total uint64
	for _, entry := range z.File {
		name := entry.Name
		if name == "" || strings.ContainsAny(name, `\\:`) || strings.HasPrefix(name, "/") {
			return fmt.Errorf("unsafe archive path")
		}
		clean := path.Clean(name)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("unsafe archive path")
		}
		for _, part := range strings.Split(clean, "/") {
			if !safeArchiveComponent(part) {
				return fmt.Errorf("unsafe archive path")
			}
		}
		key := strings.ToLower(clean)
		if seen[key] {
			return fmt.Errorf("duplicate archive path")
		}
		seen[key] = true
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return fmt.Errorf("unsupported archive entry")
		}
		if entry.UncompressedSize64 > 4<<30 || total > 8<<30-entry.UncompressedSize64 {
			return fmt.Errorf("archive exceeds size limit")
		}
		total += entry.UncompressedSize64
		target := filepath.Join(stage, filepath.FromSlash(clean))
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			reader.Close()
			return err
		}
		n, copyErr := io.Copy(file, io.LimitReader(reader, int64(entry.UncompressedSize64)+1))
		closeErr := file.Close()
		readCloseErr := reader.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if readCloseErr != nil {
			return readCloseErr
		}
		if uint64(n) != entry.UncompressedSize64 {
			return fmt.Errorf("archive entry size mismatch")
		}
	}
	return nil
}

func safeArchiveComponent(part string) bool {
	if part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, `<>"|?*`) {
		return false
	}
	for _, r := range part {
		if r < 32 {
			return false
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(part), ".")
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}
