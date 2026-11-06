package main

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unitystarter/locales"
)

// downloadJob is an editor being downloaded or installed, as the downloads
// page shows it.
type downloadJob struct {
	Entry  downloadEntry
	Path   string
	Root   string // selected when the download-and-install action starts
	Status string // downloading, downloaded, installing, installed, clearing, failedDownload, failedInstall
	Done   int64
	Total  int64
	Speed  int64
	Err    string
	cancel context.CancelFunc
}

func jobKey(e downloadEntry) string { return e.Engine + " " + e.Version }

func downloadsDir() string { return filepath.Join(userDataDir(), "downloads") }

// New Unity releases report exact sizes. Older Unity releases use a legacy
// hexadecimal MD5 encoding and rounded size estimates, as does Tuanjie's
// legacy catalog. Always verify their digest, but don't enforce the estimate.
func editorDownloadExpectedSize(e downloadEntry) int64 {
	if e.Engine == kindUnity && e.Size > 0 && !legacyUnityMD5(e.Integrity) {
		return e.Size
	}
	return 0
}

// Automatic execution is limited to a known vendor HTTPS endpoint, an
// ordinary version directory, and a usable vendor-provided integrity value.
func validateEditorInstallEntry(e downloadEntry) error {
	if e.Version == "" || parseEditorProductVersion(e.Version) != e.Version || kindOfVersion(e.Version) != e.Engine {
		return fmt.Errorf("invalid editor version or engine")
	}
	u, err := url.Parse(e.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid editor download URL")
	}
	if (e.Engine == kindUnity && u.Host != "download.unity3d.com") || (e.Engine == kindTuanjie && u.Host != "download.tuanjie.cn") {
		return fmt.Errorf("untrusted editor download host")
	}
	prefix := "UnitySetup64-"
	if e.Engine == kindTuanjie {
		prefix = "TuanjieSetup64-"
	}
	if !strings.EqualFold(filepath.Base(u.Path), prefix+e.Version+".exe") {
		return fmt.Errorf("unsupported editor installer")
	}
	h, _, err := integrityHasher(e.Integrity)
	if err != nil || h == nil {
		return fmt.Errorf("missing or invalid editor integrity")
	}
	return nil
}

func (a *app) installingEditor() bool {
	for _, job := range a.jobs {
		if job != nil && job.Status == "installing" {
			return true
		}
	}
	return false
}

// Once the scanner sees a completed installation, the installed-editor list
// is authoritative. Keeping the finished job forever would make a later
// uninstall look installed on the downloads page.
func (a *app) clearConfirmedDownloadJobs() {
	for key, job := range a.jobs {
		if job == nil || job.Status != "installed" {
			continue
		}
		for _, editor := range a.editors {
			if !editor.Simulated && editor.Kind == job.Entry.Engine && editor.Version == job.Entry.Version {
				delete(a.jobs, key)
				break
			}
		}
	}
}

func (a *app) discardDownload(e downloadEntry, job *downloadJob) {
	key := jobKey(e)
	if a.jobs[key] != job || job.Status != "failedDownload" {
		return
	}
	job.Status = "clearing"
	finish := func(err error) {
		if a.jobs[key] != job {
			return
		}
		if err != nil {
			job.Status = "failedDownload"
			a.alert(locales.OpenFailed(err.Error()))
			return
		}
		delete(a.jobs, key)
	}
	if a.win == nil {
		finish(discardResume(job.Path))
		return
	}
	win := a.win
	go func() { err := discardResume(job.Path); win.Update(func() { finish(err) }) }()
}

// installRoot is where downloaded editors go, a folder per version, as the
// hubs do.
func (a *app) installRoot() string {
	if a.prefs.InstallRoot != "" {
		return a.prefs.InstallRoot
	}
	return defaultInstallRoot()
}

func editorDownloadRoot(job *downloadJob, defaultRoot string) (string, bool) {
	if job == nil {
		return defaultRoot, true
	}
	if job.Status != "failedDownload" && job.Status != "failedInstall" {
		return "", false
	}
	if job.Root != "" {
		return job.Root, true
	}
	return defaultRoot, true
}

// startDownload fetches an installer off the main thread, reporting how much
// arrived.
func (a *app) startDownload(e downloadEntry) {
	if a.win == nil {
		return
	}
	if a.uninstallBusy {
		a.alert(locales.InstallUninstallBusy())
		return
	}
	if err := validateEditorInstallEntry(e); err != nil {
		a.alert(locales.OpenFailed(err.Error()))
		return
	}
	key := jobKey(e)
	// A retry belongs to the original operation, even if the default
	// installation directory changed while it was running.
	root, allowed := editorDownloadRoot(a.jobs[key], a.installRoot())
	if !allowed {
		return
	}
	name := filepath.Base(strings.Split(e.URL, "?")[0])
	if name == "" || name == "." || name == "/" {
		name = e.Engine + "-" + e.Version + ".exe"
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &downloadJob{Entry: e, Path: filepath.Join(downloadsDir(), name), Root: root, Status: "downloading", cancel: cancel}
	if a.jobs == nil {
		a.jobs = map[string]*downloadJob{}
	}
	a.jobs[key] = job
	win := a.win
	logger.Printf("download: %s %s from %s", e.Engine, e.Version, e.URL)
	go func() {
		last := time.Now()
		started := time.Now()
		base := int64(-1)
		err := checkEditorInstallDestination(filepath.Join(job.Root, e.Version))
		if err == nil {
			err = downloadFileVerifiedSize(ctx, e.URL, job.Path, e.Integrity, editorDownloadExpectedSize(e), func(done, total int64) {
				if base < 0 || done < base {
					base, started = done, time.Now()
				}
				if time.Since(last) < 150*time.Millisecond {
					return
				}
				last = time.Now()
				var speed int64
				if elapsed := time.Since(started).Seconds(); elapsed > 0 {
					speed = int64(float64(done-base) / elapsed)
				}
				win.Update(func() {
					job.Done, job.Total, job.Speed = done, total, speed
				})
			})
		}
		win.Update(func() { a.finishEditorDownload(key, job, err, ctx.Err(), a.installJob) })
	}()
}

func (a *app) finishEditorDownload(key string, job *downloadJob, downloadErr, contextErr error, install func(*downloadJob)) {
	if a.jobs[key] != job {
		return
	}
	if contextErr != nil {
		delete(a.jobs, key)
		return
	}
	if downloadErr != nil {
		job.Status, job.Err = "failedDownload", downloadErr.Error()
		if errors.Is(downloadErr, errDownloadStalled) {
			job.Err = locales.DownloadStalled()
		}
		logger.Printf("download: %v", downloadErr)
		return
	}
	job.Status = "downloaded"
	job.Done = job.Total
	install(job)
}

// downloadFile writes the response to a temporary file beside its place, and
// moves it there once complete, so that an interrupted download leaves no
// half an installer behind.
func downloadFile(ctx context.Context, from, to string, progress func(done, total int64)) error {
	return downloadFileVerified(ctx, from, to, "", progress)
}

func downloadFileVerified(ctx context.Context, from, to, integrity string, progress func(done, total int64)) error {
	return downloadFileVerifiedSize(ctx, from, to, integrity, 0, progress)
}

func downloadFileVerifiedSize(ctx context.Context, from, to, integrity string, expectedSize int64, progress func(done, total int64)) error {
	if expectedSize < 0 {
		return fmt.Errorf("invalid expected download size")
	}
	digest, expected, err := integrityHasher(integrity)
	if err != nil {
		return err
	}
	if digest != nil {
		if reused, err := reuseVerifiedDownload(ctx, to, expectedSize, digest, expected, progress); err != nil {
			return err
		} else if reused {
			return nil
		}
		digest.Reset()
	}
	if handled, err := parallelDownload(ctx, from, to, integrity, expectedSize, progress); handled {
		return err
	}
	watch := newDownloadIdleWatch(ctx, downloadIdleTimeout)
	defer watch.close()
	req, err := http.NewRequestWithContext(watch.ctx, http.MethodGet, from, nil)
	if err != nil {
		return err
	}
	resp, err := watch.request(http.DefaultClient, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", resp.Status)
	}
	if expectedSize > 0 && resp.ContentLength >= 0 && resp.ContentLength != expectedSize {
		return fmt.Errorf("download size mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	resumePart, statePath, lockKey, err := resumePaths(to)
	if err != nil {
		return err
	}
	release, err := acquireDownloadLock(lockKey)
	if err != nil {
		return err
	}
	defer release()
	part := resumePart + ".stream"
	if err := removeDownloadTemp(part); err != nil {
		return err
	}
	file, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(part)
	total := resp.ContentLength
	body := watch.reader(resp.Body)
	var done int64
	buf := make([]byte, 1<<20)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if expectedSize > 0 && int64(n) > expectedSize-done {
				file.Close()
				return fmt.Errorf("download exceeds expected size")
			}
			if _, err := file.Write(buf[:n]); err != nil {
				file.Close()
				os.Remove(part)
				return err
			}
			done += int64(n)
			if digest != nil {
				_, _ = digest.Write(buf[:n])
			}
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
	if expectedSize > 0 && done != expectedSize {
		return fmt.Errorf("download size mismatch")
	}
	if err := verifyIntegrity(digest, expected); err != nil {
		os.Remove(part)
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(part, to); err != nil {
		return err
	}
	return removeResumeFiles(resumePart, statePath)
}

func reuseVerifiedDownload(ctx context.Context, to string, expectedSize int64, digest hash.Hash, expected []byte, progress func(int64, int64)) (bool, error) {
	info, err := os.Lstat(to)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || (expectedSize > 0 && info.Size() != expectedSize) {
		return false, nil
	}
	file, err := os.Open(to)
	if err != nil {
		return false, nil
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return false, nil
	}
	if _, err := io.Copy(digest, &cancelableReader{ctx: ctx, reader: file}); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	if err := verifyIntegrity(digest, expected); err != nil {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	progress(info.Size(), info.Size())
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
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
	root := job.Root
	if root == "" {
		root = a.installRoot()
	}
	dest := filepath.Join(root, job.Entry.Version)
	if err := validateEditorInstallEntry(job.Entry); err != nil {
		job.Status, job.Err = "failedInstall", err.Error()
		return
	}
	job.Status = "installing"
	win := a.win
	logger.Printf("install: %s %s to %s", job.Entry.Engine, job.Entry.Version, dest)
	go func() {
		err := installVerifiedEditor(job.Entry, job.Path, dest, runEditorInstaller)
		win.Update(func() {
			if err != nil {
				job.Status = "failedInstall"
				job.Err = err.Error()
				logger.Printf("install: %v", err)
				return
			}
			job.Status = "installed"
			a.prefs.EditorRoots = dedupe(append(a.prefs.EditorRoots, root))
			a.save()
			a.refresh()
		})
	}()
}

func installVerifiedEditor(entry downloadEntry, installer, dest string, run func(string, string, string) error) error {
	return installVerifiedEditorWith(entry, installer, dest, run, verifyInstalledEditor)
}

func installVerifiedEditorWith(entry downloadEntry, installer, dest string, run func(string, string, string) error, verify func(string, string, string) error) error {
	if err := validateEditorInstallEntry(entry); err != nil {
		return err
	}
	h, expected, _ := integrityHasher(entry.Integrity)
	verified, err := reuseVerifiedDownload(context.Background(), installer, editorDownloadExpectedSize(entry), h, expected, func(int64, int64) {})
	if err != nil {
		return err
	}
	if !verified {
		return fmt.Errorf("editor installer failed integrity verification")
	}
	name := "Unity.exe"
	if entry.Engine == kindTuanjie {
		name = "Tuanjie.exe"
	}
	release, err := acquireEditorLock(filepath.Join(dest, "Editor", name))
	if err != nil {
		return err
	}
	defer release()
	if err := prepareEditorInstall(dest); err != nil {
		return err
	}
	if err := run(installer, entry.Engine, dest); err != nil {
		return err
	}
	if err := verify(entry.Engine, entry.Version, dest); err != nil {
		return err
	}
	return os.Remove(filepath.Join(dest, installMarker))
}

const installMarker = ".unitystarter-installing"

// Keep incomplete installs out of the editor list. An interrupted install can
// be retried, but a pre-existing directory is never overwritten implicitly.
func prepareEditorInstall(dest string) error {
	if err := checkEditorInstallDestination(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, installMarker), []byte("Installation incomplete; retry from UnityStarter.\n"), 0o644)
}

// Check before downloading multi-gigabyte installers and again before
// installation, since the destination may have changed in the meantime.
func checkEditorInstallDestination(dest string) error {
	if _, err := os.Stat(dest); err == nil {
		if _, markerErr := os.Stat(filepath.Join(dest, installMarker)); markerErr != nil {
			return fmt.Errorf("installation directory already exists: %s", dest)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func verifyInstalledEditor(engine, version, dest string) error {
	return verifyInstalledEditorWithVersion(engine, version, dest, editorExecutableVersion)
}

func verifyInstalledEditorWithVersion(engine, version, dest string, readVersion func(string) string) error {
	name := "Unity.exe"
	if engine == kindTuanjie {
		name = "Tuanjie.exe"
	}
	if engine != kindUnity && engine != kindTuanjie {
		return fmt.Errorf("invalid editor engine")
	}
	for _, rel := range []string{filepath.Join("Editor", name), filepath.Join("Editor", "Data", "Managed", "UnityEngine.dll"), filepath.Join("Editor", "Data", "Managed", "UnityEditor.dll")} {
		file := filepath.Join(dest, rel)
		info, err := os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("editor installation is incomplete: %s", rel)
		}
	}
	for _, rel := range []string{filepath.Join("Editor", "Data", "Resources"), filepath.Join("Editor", "Data", "PlaybackEngines")} {
		info, err := os.Stat(filepath.Join(dest, rel))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("editor installation is incomplete: %s", rel)
		}
	}
	actual := readVersion(filepath.Join(dest, "Editor", name))
	if actual == "" || !strings.EqualFold(actual, version) {
		return fmt.Errorf("editor version mismatch: expected %s, found %s", version, actual)
	}
	return nil
}
