package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A download lands whole, and reports how much arrived.
func TestDownloadFile(t *testing.T) {
	payload := bytes.Repeat([]byte("unity"), 64*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dst := filepath.Join(t.TempDir(), "editor.exe")
	var done int64
	if err := downloadFile(context.Background(), server.URL, dst, func(d, _ int64) { done = d }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("downloaded %d bytes, want %d", len(got), len(payload))
	}
	if done != int64(len(payload)) {
		t.Errorf("reported %d bytes, want %d", done, len(payload))
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("the partial file was left behind")
	}
}

func TestParallelDownloadAndIntegrity(t *testing.T) {
	payload := bytes.Repeat([]byte("unity-range-data"), (parallelMinimumSize/16)+1)
	var mu sync.Mutex
	active, maxActive, ranges := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable-version"`)
		if r.Method == http.MethodGet && r.Header.Get("Range") != "" {
			mu.Lock()
			active++
			ranges++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			time.Sleep(30 * time.Millisecond)
			defer func() { mu.Lock(); active--; mu.Unlock() }()
		}
		http.ServeContent(w, r, "installer.exe", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	hash := md5.Sum(payload)
	integrity := "md5-" + base64.StdEncoding.EncodeToString(hash[:])
	dst := filepath.Join(t.TempDir(), "editor.exe")
	var done int64
	if err := downloadFileVerified(context.Background(), server.URL, dst, integrity, func(d, _ int64) { done = d }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || !bytes.Equal(got, payload) || done != int64(len(payload)) {
		t.Fatalf("download mismatch, size %d, progress %d, error %v", len(got), done, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if ranges != parallelWorkers || maxActive < 2 {
		t.Fatalf("expected concurrent ranges, requests=%d maxActive=%d", ranges, maxActive)
	}
}

func TestParallelDownloadRejectsInvalidRanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"stable"`)
		w.Header().Set("Content-Length", fmt.Sprint(parallelMinimumSize))
		if r.Method != http.MethodHead {
			w.WriteHeader(http.StatusOK) // server ignored Range
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	dst := filepath.Join(dir, "editor.exe")
	if err := downloadFile(context.Background(), server.URL, dst, func(int64, int64) {}); err == nil {
		t.Fatal("server ignoring Range was accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files left behind: %v, %v", entries, err)
	}
}

func TestParallelDownloadRejectsBadChecksum(t *testing.T) {
	payload := bytes.Repeat([]byte("u"), parallelMinimumSize)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		http.ServeContent(w, r, "installer.exe", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	dir := t.TempDir()
	dst := filepath.Join(dir, "editor.exe")
	if err := downloadFileVerified(context.Background(), server.URL, dst, "md5-AAAAAAAAAAAAAAAAAAAAAA==", func(int64, int64) {}); err == nil {
		t.Fatal("parallel download accepted a bad checksum")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files left behind: %v, %v", entries, err)
	}
}

func TestParallelDownloadCancelCleansUp(t *testing.T) {
	payload := bytes.Repeat([]byte("u"), parallelMinimumSize)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		if r.Method == http.MethodGet {
			<-r.Context().Done()
			return
		}
		http.ServeContent(w, r, "installer.exe", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	dir := t.TempDir()
	if err := downloadFile(ctx, server.URL, filepath.Join(dir, "editor.exe"), func(int64, int64) {}); err == nil {
		t.Fatal("canceled parallel download reported success")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files left behind: %v, %v", entries, err)
	}
}

// A canceled download leaves nothing behind.
func TestDownloadFileCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	dst := filepath.Join(dir, "editor.exe")
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if err := downloadFile(ctx, server.URL, dst, func(int64, int64) {}); err == nil {
		t.Fatal("a canceled download reported success")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("files left behind: %v", entries)
	}
}

func TestDownloadRejectsWrongIntegrity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("corrupted installer"))
	}))
	defer server.Close()
	dst := filepath.Join(t.TempDir(), "editor.exe")
	if err := downloadFileVerified(context.Background(), server.URL, dst, "md5-AAAAAAAAAAAAAAAAAAAAAA==", func(int64, int64) {}); err == nil {
		t.Fatal("corrupted installer accepted")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("installer should not exist: %v", err)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial download should not exist: %v", err)
	}
}

func TestIncompleteInstallIsNotListedAndCanBeRetried(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "7000.0.0a16")
	if err := prepareEditorInstall(dest); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dest, "Editor", "Unity.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := editorsUnder(root); len(got) != 0 {
		t.Fatalf("incomplete install was listed: %v", got)
	}
	if err := prepareEditorInstall(dest); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if err := os.Remove(filepath.Join(dest, installMarker)); err != nil {
		t.Fatal(err)
	}
	if err := prepareEditorInstall(dest); err == nil {
		t.Fatal("existing editor accepted for overwrite")
	}
}

// The hubs' installers take NSIS's silent switch and the folder last.
func TestInstallerArgs(t *testing.T) {
	unity := installerArgs(kindUnity, `D:\Editors\6000.0.85f2`)
	if len(unity) != 2 || unity[0] != "/S" || unity[1] != `D:\Editors\6000.0.85f2` && unity[1] != `/D=D:\Editors\6000.0.85f2` {
		t.Errorf("unity args %v", unity)
	}
	if unity[1] != `/D=D:\Editors\6000.0.85f2` {
		t.Errorf("unity destination %q", unity[1])
	}
	tuanjie := installerArgs(kindTuanjie, `D:\Editors\2022.3.62t17`)
	if tuanjie[1] != `/D=D:\Editors\2022.3.62t17` {
		t.Errorf("tuanjie destination %q", tuanjie[1])
	}
}
