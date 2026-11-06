package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRangeStallCanResumeWithoutLosingBytes(t *testing.T) {
	payload := []byte("abcdefghij")
	var stall atomic.Bool
	stall.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "missing range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		if stall.Load() {
			_, _ = w.Write(payload[start : start+3])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		_, _ = w.Write(payload[start : end+1])
	}))
	defer server.Close()
	file, err := os.Create(filepath.Join(t.TempDir(), "partial"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := downloadRangeWithIdle(context.Background(), server.URL, file, 0, 9, 10, `"stable"`, func(int64) {}, 200*time.Millisecond)
	if got != 3 || !errors.Is(err, errDownloadStalled) {
		t.Fatalf("stall result bytes=%d err=%v", got, err)
	}
	stall.Store(false)
	got, err = downloadRangeWithIdle(context.Background(), server.URL, file, 3, 9, 10, `"stable"`, func(int64) {}, time.Second)
	if err != nil || got != 7 {
		t.Fatalf("resume bytes=%d err=%v", got, err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("resumed content=%q err=%v", data, err)
	}
}

func TestDownloadIdleWatchAllowsSlowContinuousProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 8; i++ {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer server.Close()
	watch := newDownloadIdleWatch(context.Background(), 250*time.Millisecond)
	defer watch.close()
	req, err := http.NewRequestWithContext(watch.ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := watch.request(http.DefaultClient, req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(watch.reader(resp.Body))
	if err != nil || string(data) != "xxxxxxxx" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestDownloadIdleWatchTimesOutWaitingForHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	watch := newDownloadIdleWatch(context.Background(), 200*time.Millisecond)
	defer watch.close()
	req, err := http.NewRequestWithContext(watch.ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = watch.request(http.DefaultClient, req)
	if !errors.Is(err, errDownloadStalled) {
		t.Fatalf("expected stalled request, got %v", err)
	}
}

func TestDownloadIdleWatchPreservesUserCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	watch := newDownloadIdleWatch(ctx, time.Second)
	defer watch.close()
	cancel()
	req, err := http.NewRequestWithContext(watch.ctx, http.MethodGet, "http://127.0.0.1:1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = watch.request(http.DefaultClient, req)
	if !errors.Is(err, context.Canceled) || errors.Is(err, errDownloadStalled) {
		t.Fatalf("cancellation misreported as stalled: %v", err)
	}
}
