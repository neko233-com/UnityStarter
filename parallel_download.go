package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const parallelMinimumSize = 8 << 20
const parallelWorkers = 6
const downloadUserAgent = "UnityStarter/1.0"

var errRangeIgnored = errors.New("server ignored download range")

func downloadSegmentBoundary(total int64, index int) int64 {
	return (total/parallelWorkers)*int64(index) + (total%parallelWorkers)*int64(index)/parallelWorkers
}

func rangeValidator(header http.Header) string {
	validator := header.Get("ETag")
	if strings.HasPrefix(validator, "W/") {
		validator = ""
	}
	if validator == "" {
		validator = header.Get("Last-Modified")
	}
	return validator
}

// Some CDNs reject HEAD or omit Accept-Ranges even though GET supports it.
// A one-byte GET proves range support without downloading the full file.
func probeParallelDownload(ctx context.Context, from string, expectedSize int64) (total int64, validator string, supported bool, err error) {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, from, nil)
	if err != nil {
		return 0, "", false, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", downloadUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, "", false, ctx.Err()
		}
		return 0, "", false, nil
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		total = resp.ContentLength
		if expectedSize > 0 && total >= 0 && total != expectedSize {
			return 0, "", false, fmt.Errorf("download size mismatch")
		}
		validator = rangeValidator(resp.Header)
		if total >= parallelMinimumSize && strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") && validator != "" {
			return total, validator, true, nil
		}
		if total >= 0 && total < parallelMinimumSize {
			return 0, "", false, nil
		}
	} else if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented && resp.StatusCode != http.StatusForbidden {
		return 0, "", false, nil
	}
	req, err = http.NewRequestWithContext(probeCtx, http.MethodGet, from, nil)
	if err != nil {
		return 0, "", false, err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", downloadUserAgent)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, "", false, ctx.Err()
		}
		return 0, "", false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || (resp.ContentLength >= 0 && resp.ContentLength != 1) {
		return 0, "", false, nil
	}
	prefix := "bytes 0-0/"
	contentRange := resp.Header.Get("Content-Range")
	if !strings.HasPrefix(contentRange, prefix) {
		return 0, "", false, nil
	}
	total, err = strconv.ParseInt(strings.TrimPrefix(contentRange, prefix), 10, 64)
	if err != nil || total < parallelMinimumSize {
		return 0, "", false, nil
	}
	if expectedSize > 0 && total != expectedSize {
		return 0, "", false, fmt.Errorf("download size mismatch")
	}
	validator = rangeValidator(resp.Header)
	return total, validator, validator != "", nil
}

// parallelDownload uses only the original URL. A strong validator and byte
// ranges are required, so different versions of an object cannot be mixed.
// handled=false leaves servers without range support to the streaming path.
func parallelDownload(ctx context.Context, from, to, integrity string, expectedSize int64, progress func(int64, int64)) (handled bool, err error) {
	total, validator, supported, err := probeParallelDownload(ctx, from, expectedSize)
	if err != nil {
		return true, err
	}
	if !supported {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return true, err
	}
	resumable := integrity != ""
	var file *os.File
	var part, statePath string
	var state resumeState
	var release func()
	if resumable {
		file, part, statePath, state, release, err = openResume(to, from, integrity, validator, total)
	} else {
		file, err = os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+"-*.part")
		if err == nil {
			part = file.Name()
			err = file.Truncate(total)
		}
	}
	if err != nil {
		if file != nil {
			file.Close()
			os.Remove(part)
		}
		return true, err
	}
	if release != nil {
		defer release()
	}
	cleanup := true
	defer func() {
		file.Close()
		if cleanup {
			if resumable {
				_ = removeResumeFiles(part, statePath)
			} else {
				_ = os.Remove(part)
			}
		}
	}()
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := make([]int64, parallelWorkers)
	if resumable {
		copy(done, state.Completed)
	}
	initial := append([]int64(nil), done...)
	saved := append([]int64(nil), done...)
	lastSaved := time.Now()
	var checkpointErr error
	checkpoint := func() error {
		if err := file.Sync(); err != nil {
			return err
		}
		state.Completed = append(state.Completed[:0], done...)
		if err := writeResumeState(statePath, state); err != nil {
			return err
		}
		copy(saved, done)
		lastSaved = time.Now()
		return nil
	}
	report := func(i int, n int64) {
		mu.Lock()
		defer mu.Unlock()
		done[i] = n
		var sum int64
		for _, value := range done {
			sum += value
		}
		progress(sum, total)
		if resumable && checkpointErr == nil && (n-saved[i] >= 8<<20 || time.Since(lastSaved) >= 5*time.Second) {
			if checkpointErr = checkpoint(); checkpointErr != nil {
				cancelWork()
			}
		}
	}
	if resumable {
		var resumed int64
		for _, n := range initial {
			resumed += n
		}
		if resumed > 0 {
			progress(resumed, total)
		}
	}
	for i := 0; i < parallelWorkers; i++ {
		start, end := downloadSegmentBoundary(total, i), downloadSegmentBoundary(total, i+1)-1
		wg.Add(1)
		go func(i int, start, end int64) {
			defer wg.Done()
			var chunkErr error
			completed := initial[i]
			for attempt := 0; attempt < 3; attempt++ {
				if start+completed > end {
					break
				}
				base := completed
				received, rangeErr := downloadRange(workCtx, from, file, start+base, end, total, validator, func(n int64) { report(i, base+n) })
				completed = base + received
				report(i, completed)
				chunkErr = rangeErr
				if chunkErr == nil || errors.Is(chunkErr, errRangeIgnored) || workCtx.Err() != nil {
					break
				}
				if attempt == 2 {
					break
				}
				select {
				case <-workCtx.Done():
				case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
				}
			}
			if chunkErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = chunkErr
					cancelWork()
				}
				mu.Unlock()
			}
		}(i, start, end)
	}
	wg.Wait()
	if checkpointErr != nil {
		return true, checkpointErr
	}
	if firstErr != nil {
		// A CDN may claim range support yet answer a range request with 200,
		// including when its If-Range validator has changed. Discard all
		// partial ranges and let the caller start a fresh streaming download.
		if errors.Is(firstErr, errRangeIgnored) && ctx.Err() == nil {
			progress(0, 0)
			return false, nil
		}
		if resumable && ctx.Err() == nil {
			var retained int64
			for _, n := range done {
				retained += n
			}
			if retained > 0 {
				if err := checkpoint(); err != nil {
					return true, err
				}
				cleanup = false
			}
		}
		return true, firstErr
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if integrity != "" {
		hash, expected, err := integrityHasher(integrity)
		if err != nil {
			return true, err
		}
		if _, err := io.Copy(hash, &cancelableReader{ctx: ctx, reader: io.NewSectionReader(file, 0, total)}); err != nil {
			return true, err
		}
		if err := verifyIntegrity(hash, expected); err != nil {
			return true, err
		}
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	if err := os.Rename(part, to); err != nil {
		return true, err
	}
	return true, nil
}

type cancelableReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *cancelableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type rangeProgressWriter struct {
	w      io.Writer
	n      int64
	report func(int64)
}

func (w *rangeProgressWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	w.report(w.n)
	return n, err
}

func downloadRange(ctx context.Context, from string, file *os.File, start, end, total int64, validator string, report func(int64)) (int64, error) {
	return downloadRangeWithIdle(ctx, from, file, start, end, total, validator, report, downloadIdleTimeout)
}

func downloadRangeWithIdle(ctx context.Context, from string, file *os.File, start, end, total int64, validator string, report func(int64), idle time.Duration) (int64, error) {
	watch := newDownloadIdleWatch(ctx, idle)
	defer watch.close()
	req, err := http.NewRequestWithContext(watch.ctx, http.MethodGet, from, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("If-Range", validator)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", downloadUserAgent)
	resp, err := watch.request(http.DefaultClient, req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0, errRangeIgnored
	}
	want := fmt.Sprintf("bytes %d-%d/%d", start, end, total)
	if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != want {
		return 0, fmt.Errorf("invalid range response: %s, %q", resp.Status, resp.Header.Get("Content-Range"))
	}
	if strings.HasPrefix(validator, `"`) && resp.Header.Get("ETag") != "" && resp.Header.Get("ETag") != validator {
		return 0, fmt.Errorf("installer changed during download")
	}
	length := end - start + 1
	if resp.ContentLength >= 0 && resp.ContentLength != length {
		return 0, fmt.Errorf("invalid range length")
	}
	w := &rangeProgressWriter{w: io.NewOffsetWriter(file, start), report: report}
	body := watch.reader(resp.Body)
	if _, err := io.CopyN(w, body, length); err != nil {
		return w.n, err
	}
	var extra [1]byte
	if n, err := body.Read(extra[:]); n != 0 {
		return 0, fmt.Errorf("range response exceeded expected length")
	} else if err != io.EOF {
		if err != nil {
			return w.n, err
		}
		return w.n, fmt.Errorf("range response did not terminate")
	}
	return w.n, nil
}
