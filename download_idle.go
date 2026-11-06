package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

const downloadIdleTimeout = 45 * time.Second

var errDownloadStalled = errors.New("download stalled: no response data for 45 seconds")

// A timer runs only while waiting for the server, not while processing or
// syncing a received chunk. It aborts the request without imposing a total
// deadline on a large but steadily progressing download.
type downloadIdleWatch struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	timer  *time.Timer
	idle   time.Duration
}

func newDownloadIdleWatch(parent context.Context, idle time.Duration) *downloadIdleWatch {
	ctx, cancel := context.WithCancelCause(parent)
	w := &downloadIdleWatch{ctx: ctx, cancel: cancel, idle: idle}
	w.timer = time.AfterFunc(idle, func() { cancel(errDownloadStalled) })
	return w
}

func (w *downloadIdleWatch) close() {
	w.timer.Stop()
	w.cancel(context.Canceled)
}

func (w *downloadIdleWatch) request(client *http.Client, req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req.WithContext(w.ctx))
	w.timer.Stop()
	if err != nil && w.ctx.Err() != nil {
		return nil, context.Cause(w.ctx)
	}
	return resp, err
}

func (w *downloadIdleWatch) reader(body io.Reader) io.Reader {
	return &downloadIdleReader{watch: w, body: body}
}

type downloadIdleReader struct {
	watch *downloadIdleWatch
	body  io.Reader
}

func (r *downloadIdleReader) Read(p []byte) (int, error) {
	r.watch.timer.Reset(r.watch.idle)
	n, err := r.body.Read(p)
	r.watch.timer.Stop()
	if err != nil && r.watch.ctx.Err() != nil {
		return n, context.Cause(r.watch.ctx)
	}
	return n, err
}
