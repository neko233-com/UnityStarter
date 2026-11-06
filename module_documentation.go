package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const documentationModule = "tuanjie-documentation"
const unityDocumentationModule = "unity-documentation"

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

func unityDocumentationArchive(m editorModule) (string, bool) {
	if m.ID != "documentation" || m.Parent != "" || m.Command != "" || m.EULA != "" || m.EULA2 != "" || m.Checksum != "" || m.Integrity != "" || m.RenameFrom != "" || m.RenameTo != "" || m.Destination != "{UNITY_PATH}/Editor/Data" || m.DownloadSize <= 0 || m.DownloadSize > 2<<30 {
		return "", false
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.Host != "cloudmedia-docs.unity3d.com" || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 4 || parts[1] != "docscloudstorage" || parts[3] != "UnityDocumentation.zip" {
		return "", false
	}
	version := strings.Split(parts[2], ".")
	if len(version) != 2 || version[0] == "" || version[1] == "" || strings.Trim(version[0]+version[1], "0123456789") != "" {
		return "", false
	}
	return parts[2], true
}

// The catalog size is an estimate for older releases and the archive is updated
// in place. Pin each download to the current HTTPS response's size and MD5.
// This detects transfer corruption but is not an independently signed digest.
func unityDocumentationRemote(ctx context.Context, m editorModule) (string, int64, error) {
	if _, ok := unityDocumentationArchive(m); !ok {
		return "", 0, fmt.Errorf("invalid Unity documentation metadata")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, m.URL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	return unityDocumentationResponse(resp)
}

func unityDocumentationResponse(resp *http.Response) (string, int64, error) {
	if resp.StatusCode != http.StatusOK || resp.ContentLength <= 0 || resp.ContentLength > 2<<30 {
		return "", 0, fmt.Errorf("invalid Unity documentation response")
	}
	for _, header := range resp.Header.Values("X-Goog-Hash") {
		for _, item := range strings.Split(header, ",") {
			if value, ok := strings.CutPrefix(strings.TrimSpace(item), "md5="); ok {
				if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 16 {
					return "md5-" + value, resp.ContentLength, nil
				}
			}
		}
	}
	return "", 0, fmt.Errorf("Unity documentation response has no valid MD5")
}

func documentationFilesPresent(dir string) bool {
	return documentationFilesPresentIn(dir, "zh_CN")
}

func documentationFilesPresentIn(dir, language string) bool {
	for _, section := range []string{"Manual", "ScriptReference"} {
		info, err := os.Lstat(filepath.Join(dir, language, section, "index.html"))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() == 0 {
			return false
		}
	}
	return true
}

// Extract into a sibling of the final directory, then atomically rename it.
// Existing non-empty documentation is left untouched, even on failure.
func installTuanjieDocumentation(root, archive, integrity string) error {
	return installDocumentation(root, archive, integrity, "zh_CN")
}

func installDocumentation(root, archive, integrity, language string) error {
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
	if err != nil || len(entries) != 1 || entries[0].Name() != "Documentation" || !entries[0].IsDir() || !documentationFilesPresentIn(filepath.Join(stage, "Documentation"), language) {
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
