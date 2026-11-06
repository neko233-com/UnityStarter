package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shaString(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestUpdateAssetsRejectsUntrustedOrIncompleteRelease(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	r := latestRelease{Tag: "v9.0.0", Assets: []releaseAsset{
		{Name: "unitystarter-9.0.0-amd64.msi", URL: "https://github.com/neko233-com/UnityStarter/releases/download/v9.0.0/unitystarter-9.0.0-amd64.msi", Digest: digest, Size: 100},
		{Name: "UnityStarter.exe", URL: "https://github.com/neko233-com/UnityStarter/releases/download/v9.0.0/UnityStarter.exe", Digest: digest, Size: 100},
	}}
	if _, _, err := updateAssets(r); err != nil {
		t.Fatal(err)
	}
	r.Assets[0].URL = "https://example.com/evil.msi"
	if _, _, err := updateAssets(r); err == nil {
		t.Fatal("untrusted URL accepted")
	}
	r.Assets[0].URL = "https://github.com/neko233-com/UnityStarter/releases/download/v9.0.0/unitystarter-9.0.0-amd64.msi"
	r.Assets[0].Digest = ""
	if _, _, err := updateAssets(r); err == nil {
		t.Fatal("missing digest accepted")
	}
	r.Tag = "v9.0.0-rc1"
	if _, _, err := updateAssets(r); err == nil {
		t.Fatal("prerelease accepted")
	}
}

func TestUpdateStagingAndTamperProtection(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	payload := []byte("verified MSI fixture")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	release := latestRelease{Tag: "v99.0.0"}
	msi := releaseAsset{URL: server.URL, Size: int64(len(payload)), Digest: "sha256:" + shaString(payload)}
	exe := releaseAsset{Digest: "sha256:" + shaString([]byte("exe"))}
	p, err := stageUpdate(context.Background(), server.Client(), release, msi, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := readPendingUpdate(); err != nil || got != p {
		t.Fatalf("pending %+v: %v", got, err)
	}
	if err := os.WriteFile(updateMSIPath(p.Version), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPendingUpdate(); err == nil {
		t.Fatal("tampered MSI accepted")
	}
	if err := os.Remove(pendingUpdatePath()); err != nil {
		t.Fatal(err)
	}
	msi.Digest = "sha256:" + strings.Repeat("0", 64)
	if _, err := stageUpdate(context.Background(), server.Client(), release, msi, exe); err == nil {
		t.Fatal("bad download accepted")
	}
	if _, err := os.Stat(pendingUpdatePath()); !os.IsNotExist(err) {
		t.Fatalf("invalid download created pending state: %v", err)
	}
	parts, err := filepath.Glob(filepath.Join(updatesDir(), ".update-*.part"))
	if err != nil || len(parts) != 0 {
		t.Fatalf("temporary files: %v, %v", parts, err)
	}
}

func TestPendingUpdateRejectsPathTraversal(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(updatesDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"version":"../9.0.0","msi_sha256":"%s","exe_sha256":"%s"}`, strings.Repeat("a", 64), strings.Repeat("a", 64))
	if err := os.WriteFile(pendingUpdatePath(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPendingUpdate(); err == nil {
		t.Fatal("path traversal accepted")
	}
}
