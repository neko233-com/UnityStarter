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
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelDownloadWhenHeadIsRejected(t *testing.T) {
	payload := bytes.Repeat([]byte("range-source"), parallelMinimumSize/12+1)
	var mu sync.Mutex
	active, maximum, probes, ranges := 0, 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("ETag", `"stable"`)
		mu.Lock()
		if r.Header.Get("Range") == "bytes=0-0" {
			probes++
		} else if r.Header.Get("Range") != "" {
			ranges++
			active++
			if active > maximum {
				maximum = active
			}
			defer func() { mu.Lock(); active--; mu.Unlock() }()
		}
		mu.Unlock()
		if r.Header.Get("Range") != "bytes=0-0" {
			time.Sleep(30 * time.Millisecond)
		}
		http.ServeContent(w, r, "editor.exe", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	sum := sha256.Sum256(payload)
	dest := filepath.Join(t.TempDir(), "editor.exe")
	err := downloadFileVerifiedSize(context.Background(), server.URL, dest, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]), int64(len(payload)), func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download differs: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if probes != 1 || ranges != parallelWorkers || maximum < 2 {
		t.Fatalf("probe=%d ranges=%d max concurrency=%d", probes, ranges, maximum)
	}
}

func TestDownloadSendsApplicationUserAgent(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(fmt.Sprintf("parallel=%t", parallel), func(t *testing.T) {
			size := 1024
			if parallel {
				size = parallelMinimumSize + 1024
			}
			payload := bytes.Repeat([]byte("x"), size)
			var heads, probes, ranges, streams atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.UserAgent() != downloadUserAgent {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if r.Method == http.MethodHead {
					heads.Add(1)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("ETag", `"stable"`)
				if r.Header.Get("Range") == "bytes=0-0" {
					probes.Add(1)
				} else if r.Header.Get("Range") != "" {
					ranges.Add(1)
				} else {
					streams.Add(1)
				}
				http.ServeContent(w, r, "file", time.Time{}, bytes.NewReader(payload))
			}))
			defer server.Close()
			sum := sha256.Sum256(payload)
			dest := filepath.Join(t.TempDir(), "file")
			if err := downloadFileVerifiedSize(context.Background(), server.URL, dest, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]), int64(len(payload)), func(int64, int64) {}); err != nil {
				t.Fatal(err)
			}
			if heads.Load() != 1 || probes.Load() != 1 || (parallel && ranges.Load() != parallelWorkers) || (!parallel && streams.Load() != 1) {
				t.Fatalf("heads=%d probes=%d ranges=%d streams=%d", heads.Load(), probes.Load(), ranges.Load(), streams.Load())
			}
		})
	}
}

func TestRangeProbeWhenHeadOmitsRangeSupport(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), parallelMinimumSize+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			return
		}
		http.ServeContent(w, r, "editor.exe", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	total, validator, supported, err := probeParallelDownload(context.Background(), server.URL, int64(len(payload)))
	if err != nil || !supported || total != int64(len(payload)) || validator != `"stable"` {
		t.Fatalf("range probe: total=%d validator=%q supported=%v err=%v", total, validator, supported, err)
	}
	if _, _, _, err := probeParallelDownload(context.Background(), server.URL, int64(len(payload))+1); err == nil {
		t.Fatal("wrong expected size accepted")
	}
}

func TestRangeProbeFallsBackSafely(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), parallelMinimumSize+1)
	for _, mode := range []string{"ignored-range", "no-validator", "bad-range"} {
		t.Run(mode, func(t *testing.T) {
			var probes, streamed, segments atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if r.Header.Get("Range") == "bytes=0-0" {
					probes.Add(1)
					switch mode {
					case "ignored-range":
						w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
						_, _ = w.Write(payload)
					case "no-validator":
						http.ServeContent(w, r, "editor.exe", time.Time{}, bytes.NewReader(payload))
					case "bad-range":
						w.Header().Set("Content-Range", fmt.Sprintf("bytes 1-1/%d", len(payload)))
						w.Header().Set("ETag", `"stable"`)
						w.WriteHeader(http.StatusPartialContent)
						_, _ = w.Write([]byte("x"))
					}
					return
				}
				if r.Header.Get("Range") != "" {
					segments.Add(1)
				} else {
					streamed.Add(1)
				}
				_, _ = w.Write(payload)
			}))
			defer server.Close()
			sum := sha256.Sum256(payload)
			dest := filepath.Join(t.TempDir(), "editor.exe")
			if err := downloadFileVerifiedSize(context.Background(), server.URL, dest, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]), int64(len(payload)), func(int64, int64) {}); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dest)
			if err != nil || !bytes.Equal(got, payload) || probes.Load() != 1 || streamed.Load() != 1 || segments.Load() != 0 {
				t.Fatalf("unsafe fallback: probes=%d streamed=%d segments=%d err=%v", probes.Load(), streamed.Load(), segments.Load(), err)
			}
		})
	}
}

func TestOfficialUnityDirectParallelProbe(t *testing.T) {
	if os.Getenv("UNITYSTARTER_OFFICIAL_PROBE") != "1" {
		t.Skip("set UNITYSTARTER_OFFICIAL_PROBE=1 for a direct network check")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	defer func() { http.DefaultClient = previous }()
	const source = "https://download.unity3d.com/download_unity/0745a91df592/Windows64EditorInstaller/UnitySetup64-7000.0.0a16.exe"
	total, validator, supported, err := probeParallelDownload(context.Background(), source, 3892458520)
	if err != nil || !supported || total != 3892458520 || validator == "" {
		t.Fatalf("official direct probe: size=%d supported=%v err=%v", total, supported, err)
	}
	t.Logf("official Unity 7 installer supports parallel ranges: %d bytes, proxy disabled", total)
}
