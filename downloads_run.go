package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
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
		err := downloadFileVerified(ctx, e.URL, job.Path, e.Integrity, func(done, total int64) {
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
	return downloadFileVerified(ctx, from, to, "", progress)
}

func downloadFileVerified(ctx context.Context, from, to, integrity string, progress func(done, total int64)) error {
	if integrity != "" && !strings.HasPrefix(integrity, "md5-") {
		return fmt.Errorf("unsupported installer integrity: %s", integrity)
	}
	if handled, err := parallelDownload(ctx, from, to, integrity, progress); handled {
		return err
	}
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
	file, err := os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+"-*.part")
	if err != nil {
		return err
	}
	part := file.Name()
	defer os.Remove(part)
	total := resp.ContentLength
	var done int64
	digest := md5.New() // Unity's release API supplies an MD5 integrity value.
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
			_, _ = digest.Write(buf[:n])
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
	if integrity != "" && "md5-"+base64.StdEncoding.EncodeToString(digest.Sum(nil)) != integrity {
		os.Remove(part)
		return fmt.Errorf("installer integrity mismatch")
	}
	return os.Rename(part, to)
}

// installerArgs are the arguments the hubs pass to an editor's installer:
// NSIS's silent switch, then the folder of the version, which must be last.
func installerArgs(engine, dest string) []string {
	return []string{"/S", "/D=" + dest}
}

// installJob runs a downloaded installer into the install folder, and scans
// again when it is done.
func (a *app) installJob(job *downloadJob) {
	if a.win == nil {
		return
	}
	dest := filepath.Join(a.installRoot(), job.Entry.Version)
	if err := prepareEditorInstall(dest); err != nil {
		job.Status, job.Err = "failedInstall", err.Error()
		return
	}
	job.Status = "installing"
	win := a.win
	logger.Printf("install: %s %s to %s", job.Entry.Engine, job.Entry.Version, dest)
	go func() {
		err := runEditorInstaller(job.Path, job.Entry.Engine, dest)
		if err == nil {
			err = verifyInstalledEditor(job.Entry.Engine, dest)
		}
		if err == nil {
			err = os.Remove(filepath.Join(dest, installMarker))
		}
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

const installMarker = ".unitystarter-installing"

// Keep incomplete installs out of the editor list. An interrupted install can
// be retried, but a pre-existing directory is never overwritten implicitly.
func prepareEditorInstall(dest string) error {
	if _, err := os.Stat(dest); err == nil {
		if _, markerErr := os.Stat(filepath.Join(dest, installMarker)); markerErr != nil {
			return fmt.Errorf("installation directory already exists: %s", dest)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, installMarker), []byte("Installation incomplete; retry from UnityStarter.\n"), 0o644)
}

func verifyInstalledEditor(engine, dest string) error {
	name := "Unity.exe"
	if engine == kindTuanjie {
		name = "Tuanjie.exe"
	}
	if info, err := os.Stat(filepath.Join(dest, "Editor", name)); err != nil || info.IsDir() {
		return fmt.Errorf("installer finished but %s was not found in %s", name, dest)
	}
	return nil
}
