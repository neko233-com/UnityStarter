package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// A canceled download leaves nothing behind.
func TestDownloadFileCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if tuanjie[1] != `/D="D:\Editors\2022.3.62t17"` {
		t.Errorf("tuanjie destination %q", tuanjie[1])
	}
}
