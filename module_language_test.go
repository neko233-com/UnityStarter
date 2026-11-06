package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func languageFixture() editorModule {
	return editorModule{ID: "language-zh-hans", URL: "https://new-translate.cdn.unity.cn/v1/live/54/2022.3/zh-hans", Destination: "{UNITY_PATH}/Editor/Data/Localization"}
}

func TestTuanjieLanguageInstall(t *testing.T) {
	m := languageFixture()
	if _, _, _, ok := moduleInstallSpec(editorInstall{Kind: kindTuanjie}, m); !ok {
		t.Fatal("official language rejected")
	}
	if _, _, _, ok := moduleInstallSpec(editorInstall{Kind: kindUnity}, m); ok {
		t.Fatal("language enabled for Unity")
	}
	for _, mutate := range []func(*editorModule){
		func(m *editorModule) {
			m.URL = strings.Replace(m.URL, "new-translate.cdn.unity.cn", "attacker.invalid", 1)
		},
		func(m *editorModule) { m.URL += "?x=1" },
		func(m *editorModule) { m.ID = "language-ja" },
		func(m *editorModule) { m.Destination += "/other" },
		func(m *editorModule) { m.EULA = "https://example.com" },
	} {
		bad := m
		mutate(&bad)
		if _, ok := tuanjieLanguageModule(bad); ok {
			t.Fatalf("unsafe metadata accepted: %+v", bad)
		}
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Editor", "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("msgid \"\"\nmsgstr \"\"\n\"Language: zh-hans\\n\"\n\nmsgid \"ok\"\nmsgstr \"ok\"\n")
	if err := installTuanjieLanguage(root, m, data); err != nil {
		t.Fatal(err)
	}
	if !moduleFilesPresent(root, m) {
		t.Fatal("installed file not detected")
	}
	if err := installTuanjieLanguage(root, m, data); err == nil {
		t.Fatal("existing file overwritten")
	}
	path := filepath.Join(root, "Editor", "Data", "Localization", "zh-hans.po")
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(data) {
		t.Fatal("existing file changed")
	}
	if validLanguagePO(data, "ja") || validLanguagePO([]byte("<html>error</html>"), "zh-hans") {
		t.Fatal("invalid PO accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := downloadTuanjieLanguage(ctx, m, nil); err == nil {
		t.Fatal("canceled download succeeded")
	}
}

func TestTuanjieLanguageOfficialDownload(t *testing.T) {
	if os.Getenv("UNITYSTARTER_TEST_OFFICIAL_LANGUAGE") != "1" {
		t.Skip("explicit live download only")
	}
	for _, language := range []string{"ja", "ko", "zh-hans", "zh-hant"} {
		t.Run(language, func(t *testing.T) {
			m := languageFixture()
			m.ID = "language-" + language
			m.URL = strings.TrimSuffix(m.URL, "zh-hans") + language
			data, err := downloadTuanjieLanguage(context.Background(), m, nil)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "Editor", "Data"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := installTuanjieLanguage(root, m, data); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(root, "Editor", "Data", "Localization", language+".po"))
			if err != nil || string(got) != string(data) || !moduleFilesPresent(root, m) {
				t.Fatal("official language installation mismatch")
			}
		})
	}
}
