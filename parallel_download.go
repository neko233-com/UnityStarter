package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const parallelMinimumSize = 8 << 20
const parallelWorkers = 6

// parallelDownload uses only the original URL. A strong validator and byte
// ranges are required, so different versions of an object cannot be mixed.
// handled=false leaves servers without range support to the streaming path.
func parallelDownload(ctx context.Context, from, to, integrity string, progress func(int64, int64)) (handled bool, err error) {
	probeCtx, cancelProbe := context.WithTimeout(ctx, 10*time.Second)
	defer cancelProbe()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, from, nil)
	if err != nil {
		return true, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return true, ctx.Err()
		}
		return false, nil
	}
	resp.Body.Close()
	total := resp.ContentLength
	validator := resp.Header.Get("ETag")
	if strings.HasPrefix(validator, "W/") {
		validator = ""
	}
	if validator == "" {
		validator = resp.Header.Get("Last-Modified")
	}
	if resp.StatusCode != http.StatusOK || total < parallelMinimumSize || !strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") || validator == "" {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return true, err
	}
	file, err := os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+"-*.part")
	if err != nil {
		return true, err
	}
	part := file.Name()
	defer func() { file.Close(); os.Remove(part) }()
	if err := file.Truncate(total); err != nil {
		return true, err
	}
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := make([]int64, parallelWorkers)
	report := func(i int, n int64) {
		mu.Lock()
		defer mu.Unlock()
		done[i] = n
		var sum int64
		for _, value := range done {
			sum += value
		}
		progress(sum, total)
	}
	for i := 0; i < parallelWorkers; i++ {
		start, end := total*int64(i)/parallelWorkers, total*int64(i+1)/parallelWorkers-1
		wg.Add(1)
		go func(i int, start, end int64) {
			defer wg.Done()
			var chunkErr error
			for attempt := 0; attempt < 3; attempt++ {
				report(i, 0)
				chunkErr = downloadRange(workCtx, from, file, start, end, total, validator, func(n int64) { report(i, n) })
				if chunkErr == nil || workCtx.Err() != nil {
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
	if firstErr != nil {
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
		if _, err := io.Copy(hash, io.NewSectionReader(file, 0, total)); err != nil {
			return true, err
		}
		if err := verifyIntegrity(hash, expected); err != nil {
			return true, err
		}
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	return true, os.Rename(part, to)
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

func downloadRange(ctx context.Context, from string, file *os.File, start, end, total int64, validator string, report func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("If-Range", validator)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	want := fmt.Sprintf("bytes %d-%d/%d", start, end, total)
	if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != want {
		return fmt.Errorf("invalid range response: %s, %q", resp.Status, resp.Header.Get("Content-Range"))
	}
	if strings.HasPrefix(validator, `"`) && resp.Header.Get("ETag") != "" && resp.Header.Get("ETag") != validator {
		return fmt.Errorf("installer changed during download")
	}
	length := end - start + 1
	if resp.ContentLength >= 0 && resp.ContentLength != length {
		return fmt.Errorf("invalid range length")
	}
	w := &rangeProgressWriter{w: io.NewOffsetWriter(file, start), report: report}
	if _, err := io.CopyN(w, resp.Body, length); err != nil {
		return err
	}
	var extra [1]byte
	if n, err := resp.Body.Read(extra[:]); n != 0 || err != io.EOF {
		return fmt.Errorf("range response exceeded expected length")
	}
	return nil
}
