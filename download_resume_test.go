package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadLockRejectsConcurrentWriter(t *testing.T) {
	key := downloadIdentity(t.TempDir())
	release, err := acquireDownloadLock(key)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		unlock, err := acquireDownloadLock(key)
		if err == nil {
			unlock()
		}
		result <- err
	}()
	if err := <-result; err == nil {
		release()
		t.Fatal("second writer acquired the same lock")
	}
	release()
	release, err = acquireDownloadLock(key)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	release()
}

func TestDownloadResumeRecoversInterruptedCheckpoint(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "editor.exe")
	source, integrity, validator, size := "https://example.test/editor.exe", "sha256-fixture", `"stable"`, int64(parallelMinimumSize)
	file, _, statePath, state, release, err := openResume(dest, source, integrity, validator, size)
	if err != nil {
		t.Fatal(err)
	}
	state.Completed[0] = 42
	if err := writeResumeState(statePath, state); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	release()
	if err := os.WriteFile(statePath+".tmp", []byte(`{"incomplete":`), 0o600); err != nil {
		t.Fatal(err)
	}
	file, _, _, recovered, release, err := openResume(dest, source, integrity, validator, size)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Completed[0] != 42 {
		t.Fatalf("last committed progress was lost: %+v", recovered.Completed)
	}
	if _, err := os.Lstat(statePath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("interrupted checkpoint remains: %v", err)
	}
	file.Close()
	release()
	if err := os.Mkdir(statePath+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err := openResume(dest, source, integrity, validator, size); err == nil {
		t.Fatal("unsafe temporary directory was accepted")
	}
	if info, err := os.Stat(statePath + ".tmp"); err != nil || !info.IsDir() {
		t.Fatalf("unsafe entry not preserved: %v", err)
	}
}

func TestDownloadStreamRecoversInterruptedTemporaryFile(t *testing.T) {
	payload := []byte("verified stream fixture")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "editor.exe")
	part, _, _, err := resumePaths(dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part+".stream", []byte("interrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if err := downloadFileVerified(context.Background(), server.URL, dest, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dest); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("downloaded content: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "editor.exe" {
		t.Fatalf("interrupted stream file remains: %v %v", entries, err)
	}
}

func TestParallelDownloadCancelAndDiscardRemoveResumeFiles(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), parallelMinimumSize)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		if r.Method == http.MethodHead {
			http.ServeContent(w, r, "archive", time.Time{}, bytes.NewReader(payload))
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "range", 400)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : start+4096])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	hash := sha256.Sum256(payload)
	integrity := "sha256-" + base64.StdEncoding.EncodeToString(hash[:])
	dir := t.TempDir()
	dest := filepath.Join(dir, "editor.exe")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := downloadFileVerified(ctx, server.URL, dest, integrity, func(done, total int64) {
		if done > 0 {
			cancel()
		}
	}); err == nil {
		t.Fatal("canceled download succeeded")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("cancel left files: %v %v", entries, err)
	}
	file, _, _, _, release, err := openResume(dest, server.URL, integrity, `"stable"`, int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	if err := discardResume(dest); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("discard left files: %v %v", entries, err)
	}
}

func TestDownloadCanceledOnFinalBytesDoesNotPublish(t *testing.T) {
	for _, size := range []int{1024, parallelMinimumSize} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			payload := bytes.Repeat([]byte("x"), size)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", `"stable"`)
				http.ServeContent(w, r, "archive", time.Time{}, bytes.NewReader(payload))
			}))
			defer server.Close()
			sum := sha256.Sum256(payload)
			dir := t.TempDir()
			dest := filepath.Join(dir, "editor.exe")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := downloadFileVerified(ctx, server.URL, dest, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]), func(done, total int64) {
				if total > 0 && done == total {
					cancel()
				}
			})
			if err == nil {
				t.Fatal("canceled download published")
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
				t.Fatalf("canceled download left files: %v %v", entries, err)
			}
		})
	}
}

func TestResumeCrashChild(t *testing.T) {
	if os.Getenv("UNITYSTARTER_RESUME_CRASH_CHILD") != "1" {
		t.Skip("subprocess fixture")
	}
	err := downloadFileVerified(context.Background(), os.Getenv("UNITYSTARTER_RESUME_SOURCE"), os.Getenv("UNITYSTARTER_RESUME_DEST"), os.Getenv("UNITYSTARTER_RESUME_HASH"), func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLiveOfficialParallelDownload(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_DOWNLOAD") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_DOWNLOAD=1 for the official download")
	}
	const source = "https://download.unity3d.com/download_unity/open-jdk/open-jdk-win-x64/jdk17.0.18-8_15e8817d1f5db6db3571ebe7430ef37f7fa8e60e8ff6f3e18ca1cb4c29f78774.zip"
	digest, err := hex.DecodeString("15e8817d1f5db6db3571ebe7430ef37f7fa8e60e8ff6f3e18ca1cb4c29f78774")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dest := filepath.Join(t.TempDir(), "official-jdk.zip")
	started := time.Now()
	err = downloadFileVerifiedSize(ctx, source, dest, "sha256-"+base64.StdEncoding.EncodeToString(digest), 118110508, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Size() != 118110508 {
		t.Fatalf("official file size: %v %v", info, err)
	}
	t.Logf("verified 118110508 official bytes in %s", time.Since(started))
}

func TestParallelDownloadResumesAfterProcessCrash(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), (60<<20)/16)
	sum := sha256.Sum256(payload)
	integrity := "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
	var initial atomic.Bool
	initial.Store(true)
	var downloaded atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"crash-fixture"`)
		if r.Method == http.MethodHead {
			http.ServeContent(w, r, "archive", time.Time{}, bytes.NewReader(payload))
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil || start < 0 || end < start || end >= int64(len(payload)) {
			http.Error(w, "range", http.StatusBadRequest)
			return
		}
		length := end - start + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprint(length))
		w.WriteHeader(http.StatusPartialContent)
		if initial.Load() {
			amount := int64(9 << 20)
			if amount >= length {
				amount = length - 1
			}
			_, _ = w.Write(payload[start : start+amount])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		n, _ := w.Write(payload[start : end+1])
		downloaded.Add(int64(n))
	}))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "editor.exe")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestResumeCrashChild$")
	cmd.Env = append(os.Environ(), "UNITYSTARTER_RESUME_CRASH_CHILD=1", "UNITYSTARTER_RESUME_SOURCE="+server.URL, "UNITYSTARTER_RESUME_DEST="+dest, "UNITYSTARTER_RESUME_HASH="+integrity)
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
	_, meta, _, err := resumePaths(dest)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var retained int64
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(meta); err == nil {
			var state resumeState
			if json.Unmarshal(data, &state) == nil {
				retained = 0
				for _, n := range state.Completed {
					retained += n
				}
				if retained > 0 {
					break
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if retained == 0 {
		t.Fatal("no durable progress before crash")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	initial.Store(false)
	if err := downloadFileVerified(context.Background(), server.URL, dest, integrity, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("resumed file mismatch: %v", err)
	}
	if downloaded.Load() >= int64(len(payload)) {
		t.Fatalf("crash resume downloaded entire file: %d", downloaded.Load())
	}
	t.Logf("durable bytes=%d, downloaded after crash=%d, total=%d", retained, downloaded.Load(), len(payload))
}

func TestParallelDownloadResumesAfterNetworkFailure(t *testing.T) {
	for _, mode := range []string{"resume", "changed-validator", "corrupt-partial"} {
		t.Run(mode, func(t *testing.T) {
			payload := bytes.Repeat([]byte("range-fixture-123"), (parallelMinimumSize/17)+2)
			var failing atomic.Bool
			failing.Store(true)
			var received atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				etag := `"first"`
				if !failing.Load() && mode == "changed-validator" {
					etag = `"second"`
				}
				w.Header().Set("ETag", etag)
				if r.Method == http.MethodHead {
					http.ServeContent(w, r, "archive", time.Time{}, bytes.NewReader(payload))
					return
				}
				var start, end int64
				if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil || start < 0 || end < start || end >= int64(len(payload)) {
					http.Error(w, "range", http.StatusBadRequest)
					return
				}
				length := end - start + 1
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
				w.Header().Set("Content-Length", fmt.Sprint(length))
				w.WriteHeader(http.StatusPartialContent)
				if failing.Load() {
					_, _ = w.Write(payload[start : start+length/4])
					return
				}
				n, _ := w.Write(payload[start : end+1])
				received.Add(int64(n))
			}))
			defer server.Close()
			hash := sha256.Sum256(payload)
			integrity := "sha256-" + base64.StdEncoding.EncodeToString(hash[:])
			dir := t.TempDir()
			dest := filepath.Join(dir, "editor.exe")
			if err := downloadFileVerified(context.Background(), server.URL, dest, integrity, func(int64, int64) {}); err == nil {
				t.Fatal("interrupted download reported success")
			}
			part, meta, _, err := resumePaths(dest)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(meta)
			if err != nil {
				t.Fatalf("missing progress: %v", err)
			}
			var state resumeState
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatal(err)
			}
			var retained int64
			for _, n := range state.Completed {
				retained += n
			}
			if retained <= 0 {
				t.Fatal("no completed data retained")
			}
			if mode == "corrupt-partial" {
				file, err := os.OpenFile(part, os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				for i, n := range state.Completed {
					if n > 0 {
						_, err = file.WriteAt([]byte{0}, int64(len(payload))*int64(i)/parallelWorkers)
						break
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			failing.Store(false)
			err = downloadFileVerified(context.Background(), server.URL, dest, integrity, func(int64, int64) {})
			if mode == "corrupt-partial" {
				if err == nil {
					t.Fatal("corrupted retained data was accepted")
				}
				if _, err := os.Stat(dest); !os.IsNotExist(err) {
					t.Fatalf("corrupted destination exists: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got, err := os.ReadFile(dest); err != nil || !bytes.Equal(got, payload) {
					t.Fatalf("destination mismatch: %v", err)
				}
				if mode == "resume" && received.Load() >= int64(len(payload)) {
					t.Fatalf("no bytes saved: received=%d total=%d", received.Load(), len(payload))
				}
				if mode == "changed-validator" && received.Load() != int64(len(payload)) {
					t.Fatalf("stale data reused: received=%d total=%d", received.Load(), len(payload))
				}
			}
			if _, err := os.Stat(part); !os.IsNotExist(err) {
				t.Fatalf("partial remains: %v", err)
			}
			if _, err := os.Stat(meta); !os.IsNotExist(err) {
				t.Fatalf("progress remains: %v", err)
			}
		})
	}
}
