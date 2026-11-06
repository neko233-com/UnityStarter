package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func androidCacheFixture(source string) (androidInstallPlan, []byte, []byte) {
	first := bytes.Repeat([]byte("a"), 4096)
	second := bytes.Repeat([]byte("b"), parallelMinimumSize)
	plan := androidInstallPlan{Module: editorModule{ID: "android-sdk-ndk-tools"}, License: "fixture terms"}
	for i, body := range [][]byte{first, second} {
		sum := sha256.Sum256(body)
		plan.Packages = append(plan.Packages, androidPackage{Module: editorModule{ID: fmt.Sprintf("package-%d", i), URL: source + []string{"/one", "/two"}[i]}, Integrity: "sha256-" + base64.StdEncoding.EncodeToString(sum[:]), ExpectedSize: int64(len(body))})
	}
	return plan, first, second
}

func TestAndroidCacheRejectsUnownedFilesAndSeparatesPlans(t *testing.T) {
	root, base := t.TempDir(), t.TempDir()
	plan, _, _ := androidCacheFixture("http://example.test")
	key, _, err := androidCacheIdentity(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	preview := plan
	preview.CachedBytes = 123
	preview.RecoveryPending = true
	if previewKey, _, err := androidCacheIdentity(root, preview); err != nil || previewKey != key {
		t.Fatalf("UI state changed cache identity: %s %v", previewKey, err)
	}
	cache, err := openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(cache.dir, "user-note.txt")
	if err := os.WriteFile(foreign, []byte("keep"), 0o600); err != nil {
		cache.close()
		t.Fatal(err)
	}
	if err := cache.clear(); err == nil {
		cache.close()
		t.Fatal("unowned file was deleted")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "keep" {
		cache.close()
		t.Fatalf("unowned file changed: %q %v", data, err)
	}
	other := plan
	other.License = "different terms"
	result := make(chan error, 1)
	go func() {
		second, err := openAndroidCache(base, root, other)
		if second != nil {
			second.close()
		}
		result <- err
	}()
	if err := <-result; err == nil {
		cache.close()
		t.Fatal("concurrent plan acquired editor lock")
	}
	oldDir := cache.dir
	cache.close()
	second, err := openAndroidCache(base, root, other)
	if err != nil {
		t.Fatal(err)
	}
	if second.dir == oldDir {
		second.close()
		t.Fatal("changed license reused old cache")
	}
	if err := second.clear(); err != nil {
		second.close()
		t.Fatal(err)
	}
	second.close()
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "keep" {
		t.Fatalf("unowned file changed: %q %v", data, err)
	}
}

func TestAndroidCacheRetainsOnFailureAndClearsOnCancel(t *testing.T) {
	root, base := t.TempDir(), t.TempDir()
	plan, first, _ := androidCacheFixture("http://example.test")
	cache, err := openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	dir := cache.dir
	if err := os.WriteFile(filepath.Join(dir, "000.zip"), first, 0o600); err != nil {
		cache.close()
		t.Fatal(err)
	}
	retained, err := finishAndroidCache(cache, root, plan, false, context.DeadlineExceeded)
	if err != nil || retained != int64(len(first)) {
		t.Fatalf("failed cache not retained: %d %v", retained, err)
	}
	cache, err = openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finishAndroidCache(cache, root, plan, false, context.Canceled); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("canceled cache remains: %v", err)
	}
}

func TestAndroidCacheReusesCompletedPackageAfterNetworkFailure(t *testing.T) {
	root, base := t.TempDir(), t.TempDir()
	_, first, second := androidCacheFixture("")
	var failing atomic.Bool
	failing.Store(true)
	var firstGets atomic.Int32
	var cachePath atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"android-failure-fixture"`)
		if r.URL.Path == "/one" {
			if r.Method == http.MethodGet {
				firstGets.Add(1)
			}
			http.ServeContent(w, r, "one", time.Time{}, bytes.NewReader(first))
			return
		}
		if r.Method == http.MethodGet && failing.Load() {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(filepath.Join(cachePath.Load().(string), "000.zip")); err == nil {
					break
				}
				select {
				case <-r.Context().Done():
					return
				case <-time.After(20 * time.Millisecond):
				}
			}
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		http.ServeContent(w, r, "two", time.Time{}, bytes.NewReader(second))
	}))
	defer server.Close()
	plan, _, _ := androidCacheFixture(server.URL)
	cache, err := openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	cachePath.Store(cache.dir)
	_, err = downloadAndroidPackages(context.Background(), plan, cache.dir, func(int64, int64) {})
	if err == nil {
		cache.close()
		t.Fatal("failed package reported success")
	}
	retained, err := finishAndroidCache(cache, root, plan, false, nil)
	if err != nil || retained < int64(len(first)) {
		t.Fatalf("completed package not retained: %d %v", retained, err)
	}
	failing.Store(false)
	cache, err = openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	if _, err = downloadAndroidPackages(context.Background(), plan, cache.dir, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if firstGets.Load() != 1 {
		t.Fatalf("completed package downloaded %d times", firstGets.Load())
	}
	if _, err := finishAndroidCache(cache, root, plan, true, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAndroidCacheChild(t *testing.T) {
	if os.Getenv("UNITYSTARTER_ANDROID_CACHE_CHILD") != "1" {
		t.Skip("subprocess fixture")
	}
	plan, _, _ := androidCacheFixture(os.Getenv("UNITYSTARTER_ANDROID_CACHE_SOURCE"))
	cache, err := openAndroidCache(os.Getenv("UNITYSTARTER_ANDROID_CACHE_BASE"), os.Getenv("UNITYSTARTER_ANDROID_CACHE_ROOT"), plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	_, err = downloadAndroidPackages(context.Background(), plan, cache.dir, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAndroidCacheReusesCompletedPackageAfterCrash(t *testing.T) {
	root, base := t.TempDir(), t.TempDir()
	var stalled atomic.Bool
	stalled.Store(true)
	var firstGets atomic.Int32
	_, first, second := androidCacheFixture("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"android-cache-fixture"`)
		if r.URL.Path == "/one" {
			if r.Method == http.MethodGet {
				firstGets.Add(1)
			}
			http.ServeContent(w, r, "one", time.Time{}, bytes.NewReader(first))
			return
		}
		if r.Method == http.MethodHead || !stalled.Load() {
			http.ServeContent(w, r, "two", time.Time{}, bytes.NewReader(second))
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil || start < 0 || end < start || end >= int64(len(second)) {
			http.Error(w, "range", 400)
			return
		}
		length := end - start + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(second)))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
		amount := int64(65536)
		if amount >= length {
			amount = length - 1
		}
		_, _ = w.Write(second[start : start+amount])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	plan, _, _ := androidCacheFixture(server.URL)
	_, dir, _, err := androidCacheLocation(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAndroidCacheChild$")
	cmd.Env = append(os.Environ(), "UNITYSTARTER_ANDROID_CACHE_CHILD=1", "UNITYSTARTER_ANDROID_CACHE_SOURCE="+server.URL, "UNITYSTARTER_ANDROID_CACHE_BASE="+base, "UNITYSTARTER_ANDROID_CACHE_ROOT="+root)
	hideWindowCmd(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(filepath.Join(dir, "000.zip")); err == nil && info.Size() == int64(len(first)) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "000.zip")); err != nil {
		t.Fatal("first package did not finish before crash")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	stalled.Store(false)
	cache, err := openAndroidCache(base, root, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.close()
	if got := androidCachedBytes(base, root, plan); got < int64(len(first)) {
		t.Fatalf("cached bytes=%d", got)
	}
	paths, err := downloadAndroidPackages(context.Background(), plan, cache.dir, func(int64, int64) {})
	if err != nil || len(paths) != 2 {
		t.Fatalf("resume: %v %v", paths, err)
	}
	if firstGets.Load() != 1 {
		t.Fatalf("completed package downloaded %d times", firstGets.Load())
	}
	if err := cache.clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cache remains: %v", err)
	}
}
