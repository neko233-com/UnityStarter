package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectGit(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.MkdirAll(git, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = https://github.com/a/b.git\n"
	if err := os.WriteFile(filepath.Join(git, "config"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	v := detectVCS(dir)
	if v.Kind != "git" || v.Branch != "main" || v.Remote != "https://github.com/a/b.git" {
		t.Errorf("git %+v", v)
	}
}

func TestDetectSvnAndLore(t *testing.T) {
	svn := t.TempDir()
	if err := os.MkdirAll(filepath.Join(svn, ".svn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v := detectVCS(svn); v.Kind != "svn" {
		t.Errorf("svn %+v", v)
	}
	lore := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lore, ".lore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v := detectVCS(lore); v.Kind != "lore" {
		t.Errorf("lore %+v", v)
	}
	if v := detectVCS(t.TempDir()); v.Kind != "" {
		t.Errorf("plain folder %+v", v)
	}
}

func TestDetectPipeline(t *testing.T) {
	cases := map[string]string{
		`{"dependencies":{"com.unity.render-pipelines.universal":"14.0.0"}}`:       "urp",
		`{"dependencies":{"com.unity.render-pipelines.high-definition":"14.0.0"}}`: "hdrp",
		`{"dependencies":{"com.unity.modules.ui":"1.0.0"}}`:                        "builtin",
	}
	for manifest, want := range cases {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "Packages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Packages", "manifest.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := detectPipeline(dir); got != want {
			t.Errorf("%s: %q, want %q", manifest, got, want)
		}
	}
	if got := detectPipeline(t.TempDir()); got != "" {
		t.Errorf("no manifest: %q", got)
	}
}

func TestTuanjieReleases(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "TuanjieHub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"official":[{"version":"2022.3.61t15","downloadUrl":"https://download.tuanjie.cn/x.exe","downloadSize":100,"lts":true}],"beta":[]}`
	if err := os.WriteFile(filepath.Join(dir, "releases.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := tuanjieReleasesIn([]string{dir})
	if len(got) != 1 {
		t.Fatalf("releases %v", got)
	}
	if got[0].Version != "2022.3.61t15" || !got[0].LTS || got[0].Size != 100 || got[0].Engine != kindTuanjie {
		t.Errorf("release %+v", got[0])
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:          "",
		512:        "512 B",
		1024:       "1.0 KB",
		3959954624: "3.7 GB",
	}
	for bytes, want := range cases {
		if got := humanSize(bytes); got != want {
			t.Errorf("%d: %q, want %q", bytes, got, want)
		}
	}
}

// Old rotated logs are forgotten, and only the newest kept.
func TestPruneLogs(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 7; i++ {
		path := filepath.Join(dir, fmt.Sprintf("app.log.2026010%d", i))
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-time.Duration(i) * 24 * time.Hour)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	pruneLogs(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	rotated := 0
	for _, entry := range entries {
		if entry.Name() != "app.log" {
			rotated++
		}
	}
	if rotated != logKeep {
		t.Errorf("kept %d rotated logs, want %d", rotated, logKeep)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.log")); err != nil {
		t.Errorf("the current log was removed: %v", err)
	}
}
