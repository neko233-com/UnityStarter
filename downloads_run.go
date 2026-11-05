package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// downloadJob is an editor being downloaded or installed, as the downloads
// page shows it.
type downloadJob struct {
	Entry  downloadEntry
	Path   string
	Status string // downloading, downloaded, installing, installed, failedDownload, failedInstall
	Done   int64
	Total  int64
	Err    string
	cancel context.CancelFunc
}

func jobKey(e downloadEntry) string { return e.Engine + " " + e.Version }

func downloadsDir() string { return filepath.Join(userDataDir(), "downloads") }

// installRoot is where downloaded editors go, a folder per version, as the
// hubs do.
func (a *app) installRoot() string {
	if a.prefs.InstallRoot != "" {
		return a.prefs.InstallRoot
	}
	return defaultInstallRoot()
}

// startDownload fetches an installer off the main thread, reporting how much
// arrived.
func (a *app) startDownload(e downloadEntry) {
	if a.win == nil {
		return
	}
	key := jobKey(e)
	if job, ok := a.jobs[key]; ok && (job.Status == "downloading" || job.Status == "installing") {
		return
	}
	name := filepath.Base(strings.Split(e.URL, "?")[0])
	if name == "" || name == "." || name == "/" {
		name = e.Engine + "-" + e.Version + ".exe"
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &downloadJob{Entry: e, Path: filepath.Join(downloadsDir(), name), Status: "downloading", cancel: cancel}
	if a.jobs == nil {
		a.jobs = map[string]*downloadJob{}
	}
	a.jobs[key] = job
	win := a.win
	logger.Printf("download: %s %s from %s", e.Engine, e.Version, e.URL)
	go func() {
		last := time.Now()
		err := downloadFile(ctx, e.URL, job.Path, func(done, total int64) {
			if time.Since(last) < 150*time.Millisecond {
				return
			}
			last = time.Now()
			win.Update(func() {
				job.Done, job.Total = done, total
			})
		})
		win.Update(func() {
			switch {
			case err == nil:
				job.Status = "downloaded"
				job.Done = job.Total
			case ctx.Err() != nil:
				delete(a.jobs, key)
			default:
				job.Status = "failedDownload"
				job.Err = err.Error()
				logger.Printf("download: %v", err)
			}
		})
	}()
}

// downloadFile writes the response to a temporary file beside its place, and
// moves it there once complete, so that an interrupted download leaves no
// half an installer behind.
func downloadFile(ctx context.Context, from, to string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	part := to + ".part"
	file, err := os.Create(part)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 1<<20)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				file.Close()
				os.Remove(part)
				return err
			}
			done += int64(n)
			progress(done, total)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			file.Close()
			os.Remove(part)
			return readErr
		}
	}
	if err := file.Close(); err != nil {
		os.Remove(part)
		return err
	}
	return os.Rename(part, to)
}

// installerArgs are the arguments the hubs pass to an editor's installer:
// NSIS's silent switch, then the folder of the version, which must be last.
func installerArgs(engine, dest string) []string {
	if engine == kindTuanjie {
		return []string{"/S", `/D="` + dest + `"`}
	}
	return []string{"/S", "/D=" + dest}
}

// installJob runs a downloaded installer into the install folder, and scans
// again when it is done.
func (a *app) installJob(job *downloadJob) {
	if a.win == nil {
		return
	}
	dest := filepath.Join(a.installRoot(), job.Entry.Version)
	args := installerArgs(job.Entry.Engine, dest)
	job.Status = "installing"
	win := a.win
	logger.Printf("install: %s %s to %s", job.Entry.Engine, job.Entry.Version, dest)
	go func() {
		cmd := exec.Command(job.Path, args...)
		hideWindowCmd(cmd)
		err := cmd.Run()
		win.Update(func() {
			if err != nil {
				job.Status = "failedInstall"
				job.Err = err.Error()
				logger.Printf("install: %v", err)
				return
			}
			job.Status = "installed"
			a.prefs.EditorRoots = dedupe(append(a.prefs.EditorRoots, a.installRoot()))
			a.save()
			a.refresh()
		})
	}()
}
