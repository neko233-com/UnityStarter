package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const documentationModule = "tuanjie-documentation"

// Tuanjie publishes this archive with an MD5 in its official module list.
// Limit this handler to the known archive, destination and editor version.
func tuanjieDocumentationArchive(m editorModule) (integrity, version string, ok bool) {
	if m.ID != "documentation" || m.Parent != "" || m.Command != "" || m.EULA != "" || m.EULA2 != "" || m.Destination != "{UNITY_PATH}/Editor/Data" || m.DownloadSize <= 0 || m.DownloadSize > 2<<30 {
		return "", "", false
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.Host != "download.tuanjie.cn" || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "download_docs" || parts[3] != "TuanjieDocumentation.zip" || parseEditorProductVersion(parts[2]) != parts[2] {
		return "", "", false
	}
	digest, err := hex.DecodeString(m.Checksum)
	if err != nil || len(digest) != 16 {
		return "", "", false
	}
	return "md5-" + base64.StdEncoding.EncodeToString(digest), parts[2], true
}

func documentationFilesPresent(dir string) bool {
	for _, rel := range []string{"zh_CN/Manual/index.html", "zh_CN/ScriptReference/index.html"} {
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() == 0 {
			return false
		}
	}
	return true
}

// Extract into a sibling of the final directory, then atomically rename it.
// Existing non-empty documentation is left untouched, even on failure.
func installTuanjieDocumentation(root, archive, integrity string) error {
	if !strings.HasPrefix(integrity, "md5-") {
		return fmt.Errorf("invalid documentation integrity")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for _, part := range []string{"Editor", "Data"} {
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("documentation parent is missing or unsafe")
		}
	}
	dest := filepath.Join(parent, "Documentation")
	checkDestination := func() (bool, error) {
		info, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("documentation destination is unsafe")
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			return false, err
		}
		if len(entries) > 0 {
			return false, fmt.Errorf("documentation destination already contains files")
		}
		return true, nil
	}
	if _, err := checkDestination(); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".unitystarter-docs-")
	if err != nil {
		return err
	}
	defer removeAndroidStage(parent, stage, ".unitystarter-docs-")
	if err := extractVerifiedArchiveWithMD5(archive, stage, integrity, 100000, true); err != nil {
		return err
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 1 || entries[0].Name() != "Documentation" || !entries[0].IsDir() || !documentationFilesPresent(filepath.Join(stage, "Documentation")) {
		return fmt.Errorf("documentation archive is incomplete or has unexpected files")
	}
	empty, err := checkDestination()
	if err != nil {
		return err
	}
	if empty {
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	if err := os.Rename(filepath.Join(stage, "Documentation"), dest); err != nil {
		if empty {
			_ = os.Mkdir(dest, 0o755)
		}
		return err
	}
	return nil
}
