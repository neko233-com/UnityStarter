package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const languageModule = "editor-language"
const maxLanguageBytes = 10 << 20

var languageURL = regexp.MustCompile(`^/v1/live/[0-9]+/[0-9]+\.[0-9]+/(ja|ko|zh-hans|zh-hant)$`)

type languageProgressReader struct {
	reader      io.Reader
	done, total int64
	progress    func(int64, int64)
}

func (r *languageProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.done += int64(n)
	if n > 0 && r.progress != nil {
		r.progress(r.done, r.total)
	}
	return n, err
}

// Both official Hubs place the .po file in Localization.
// These endpoints do not publish a checksum; TLS and PO validation provide
// transport and format checks, not cryptographic authenticity of the file.
func editorLanguageModule(m editorModule) (language, kind string, ok bool) {
	if m.Parent != "" || m.Command != "" || m.EULA != "" || m.EULA2 != "" || m.Checksum != "" || m.Integrity != "" || m.RenameFrom != "" || m.RenameTo != "" || m.Destination != "{UNITY_PATH}/Editor/Data/Localization" {
		return "", "", false
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	switch u.Host {
	case "new-translate.cdn.unity.cn":
		kind = kindTuanjie
	case "new-translate.unity3d.jp":
		kind = kindUnity
	default:
		return "", "", false
	}
	match := languageURL.FindStringSubmatch(u.Path)
	if len(match) != 2 || m.ID != "language-"+match[1] {
		return "", "", false
	}
	return match[1], kind, true
}

func validLanguagePO(data []byte, language string) bool {
	if len(data) == 0 || len(data) > maxLanguageBytes || !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return false
	}
	header := string(data)
	if i := strings.Index(header, "\n\n"); i >= 0 {
		header = header[:i]
	}
	return strings.HasPrefix(header, "msgid \"\"\nmsgstr \"\"\n") && strings.Contains(header, `"Language: `+language+`\n"`)
}

func downloadEditorLanguage(ctx context.Context, m editorModule, progress func(int64, int64)) ([]byte, error) {
	language, _, ok := editorLanguageModule(m)
	if !ok {
		return nil, fmt.Errorf("unsupported language metadata")
	}
	source, _ := url.Parse(m.URL)
	sourceParts := strings.Split(source.Path, "/")
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != source.Host || req.URL.User != nil || req.URL.RawPath != "" || req.URL.RawQuery != "" || req.URL.Fragment != "" {
			return fmt.Errorf("unsafe language redirect")
		}
		parts := strings.Split(req.URL.Path, "/")
		if len(parts) != 4 || parts[1] != "localized_editor_ui" || parts[2] != sourceParts[4] || parts[3] != language+".po" {
			return fmt.Errorf("unexpected language redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxLanguageBytes {
		return nil, fmt.Errorf("invalid language response")
	}
	reader := &languageProgressReader{reader: io.LimitReader(resp.Body, maxLanguageBytes+1), total: resp.ContentLength, progress: progress}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if !validLanguagePO(data, language) {
		return nil, fmt.Errorf("invalid language file")
	}
	return data, nil
}

func installEditorLanguage(root string, m editorModule, data []byte) error {
	language, _, ok := editorLanguageModule(m)
	if !ok || !validLanguagePO(data, language) {
		return fmt.Errorf("invalid language module")
	}
	parent, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for _, part := range []string{"Editor", "Data"} {
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("language parent is missing or unsafe")
		}
	}
	parent = filepath.Join(parent, "Localization")
	if err := os.Mkdir(parent, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("language destination is unsafe")
	}
	file, err := os.CreateTemp(parent, ".unitystarter-language-*")
	if err != nil {
		return err
	}
	stage := file.Name()
	defer os.Remove(stage)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	// Linking does not replace an existing file, including when another process
	// creates the destination between our check and commit.
	return os.Link(stage, filepath.Join(parent, language+".po"))
}
