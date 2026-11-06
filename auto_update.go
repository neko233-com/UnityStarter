package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

var stableUpdateTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

type pendingUpdate struct {
	Version   string `json:"version"`
	MSISHA256 string `json:"msi_sha256"`
	ExeSHA256 string `json:"exe_sha256"`
}

func updatesDir() string        { return filepath.Join(userDataDir(), "updates") }
func pendingUpdatePath() string { return filepath.Join(updatesDir(), "pending.json") }
func updateFailurePath() string { return filepath.Join(updatesDir(), "failure.json") }
func updateMSIPath(version string) string {
	return filepath.Join(updatesDir(), "unitystarter-"+version+"-amd64.msi")
}

func validSHA256(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size && value == strings.ToLower(value)
}

// updateAssets accepts only stable versions and the exact assets on the
// official GitHub release. The metadata's digest is required for both files.
func updateAssets(release latestRelease) (releaseAsset, releaseAsset, error) {
	if !stableUpdateTag.MatchString(release.Tag) {
		return releaseAsset{}, releaseAsset{}, fmt.Errorf("invalid release tag")
	}
	version := strings.TrimPrefix(release.Tag, "v")
	var msi, exe releaseAsset
	for _, asset := range release.Assets {
		if asset.Name == "unitystarter-"+version+"-amd64.msi" {
			msi = asset
		}
		if asset.Name == "UnityStarter.exe" {
			exe = asset
		}
	}
	for _, asset := range []releaseAsset{msi, exe} {
		u, err := url.Parse(asset.URL)
		want := "/neko233-com/UnityStarter/releases/download/" + release.Tag + "/" + asset.Name
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.Path != want || u.RawQuery != "" || u.Fragment != "" || asset.Size <= 0 || asset.Size > 100<<20 || !strings.HasPrefix(asset.Digest, "sha256:") || !validSHA256(strings.TrimPrefix(asset.Digest, "sha256:")) {
			return releaseAsset{}, releaseAsset{}, fmt.Errorf("release asset missing or invalid: %s", asset.Name)
		}
	}
	return msi, exe, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// stageUpdate downloads to a temporary file and publishes pending.json only
// after its complete size and digest have been verified.
func stageUpdate(ctx context.Context, client *http.Client, release latestRelease, msi, exe releaseAsset) (pendingUpdate, error) {
	version := strings.TrimPrefix(release.Tag, "v")
	if !stableUpdateTag.MatchString(release.Tag) || !newerVersion(release.Tag) || msi.Size <= 0 || msi.Size > 100<<20 || !strings.HasPrefix(msi.Digest, "sha256:") || !validSHA256(strings.TrimPrefix(msi.Digest, "sha256:")) || !strings.HasPrefix(exe.Digest, "sha256:") || !validSHA256(strings.TrimPrefix(exe.Digest, "sha256:")) {
		return pendingUpdate{}, fmt.Errorf("invalid release tag")
	}
	if err := os.MkdirAll(updatesDir(), 0o700); err != nil {
		return pendingUpdate{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, msi.URL, nil)
	if err != nil {
		return pendingUpdate{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		// Redirect URLs may contain short-lived credentials; never log them.
		return pendingUpdate{}, fmt.Errorf("update download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return pendingUpdate{}, fmt.Errorf("update download: %s", resp.Status)
	}
	f, err := os.CreateTemp(updatesDir(), ".update-*.part")
	if err != nil {
		return pendingUpdate{}, err
	}
	part := f.Name()
	defer os.Remove(part)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, msi.Size+1))
	closeErr := f.Close()
	if copyErr != nil {
		return pendingUpdate{}, copyErr
	}
	if closeErr != nil {
		return pendingUpdate{}, closeErr
	}
	if n != msi.Size || hex.EncodeToString(h.Sum(nil)) != strings.TrimPrefix(msi.Digest, "sha256:") {
		return pendingUpdate{}, fmt.Errorf("update checksum or size mismatch")
	}
	pending := pendingUpdate{Version: version, MSISHA256: strings.TrimPrefix(msi.Digest, "sha256:"), ExeSHA256: strings.TrimPrefix(exe.Digest, "sha256:")}
	if err := os.Rename(part, updateMSIPath(version)); err != nil {
		return pendingUpdate{}, err
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return pendingUpdate{}, err
	}
	if err := writeFileAtomic(pendingUpdatePath(), data); err != nil {
		return pendingUpdate{}, err
	}
	return pending, nil
}

func readPendingUpdate() (pendingUpdate, error) {
	data, err := os.ReadFile(pendingUpdatePath())
	if err != nil {
		return pendingUpdate{}, err
	}
	var p pendingUpdate
	if json.Unmarshal(data, &p) != nil || !stableUpdateTag.MatchString("v"+p.Version) || !validSHA256(p.MSISHA256) || !validSHA256(p.ExeSHA256) || !newerVersion("v"+p.Version) {
		return pendingUpdate{}, fmt.Errorf("invalid pending update")
	}
	digest, err := fileSHA256(updateMSIPath(p.Version))
	if err != nil || digest != p.MSISHA256 {
		return pendingUpdate{}, fmt.Errorf("pending update failed verification")
	}
	return p, nil
}

func updateHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || (req.URL.Hostname() != "github.com" && req.URL.Hostname() != "release-assets.githubusercontent.com" && req.URL.Hostname() != "objects.githubusercontent.com") {
			return fmt.Errorf("untrusted update redirect")
		}
		return nil
	}}
}

type updateFailure struct {
	Version string `json:"version"`
	Message string `json:"message"`
}

func readUpdateFailure() updateFailure {
	var failure updateFailure
	if data, err := os.ReadFile(updateFailurePath()); err == nil {
		_ = json.Unmarshal(data, &failure)
	}
	return failure
}

func recordUpdateFailure(version string, err error) {
	_ = os.MkdirAll(updatesDir(), 0o700)
	data, _ := json.Marshal(updateFailure{Version: version, Message: err.Error()})
	_ = writeFileAtomic(updateFailurePath(), data)
}

func (a *app) startAutomaticUpdates() {
	if a.updateCtx == nil {
		a.updateCtx, a.updateCancel = context.WithCancel(context.Background())
	}
	if failure := readUpdateFailure(); failure.Message != "" {
		a.updateStatus = locales.UpdateFailed(failure.Message)
	}
	a.queueUpdateCheck(false)
	win, ctx := a.win, a.updateCtx
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				win.Update(func() { a.queueUpdateCheck(false) })
			}
		}
	}()
}

func (a *app) queueUpdateCheck(manual bool) {
	if a.updateBusy || a.win == nil || (!manual && a.updatePending != "") {
		return
	}
	a.updateBusy = true
	a.updateStatus = locales.UpdateChecking()
	win, base := a.win, a.updateCtx
	if base == nil {
		base = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, 5*time.Minute)
		defer cancel()
		release, err := checkLatestRelease(ctx)
		var pending pendingUpdate
		if err == nil && newerVersion(release.Tag) {
			failure := readUpdateFailure()
			if !manual && failure.Version == strings.TrimPrefix(release.Tag, "v") && failure.Message != "" {
				err = fmt.Errorf("%s", failure.Message)
			} else if existing, readErr := readPendingUpdate(); readErr == nil && existing.Version == strings.TrimPrefix(release.Tag, "v") {
				pending = existing
			} else {
				var msi, exe releaseAsset
				msi, exe, err = updateAssets(release)
				if err == nil {
					pending, err = stageUpdate(ctx, updateHTTPClient(), release, msi, exe)
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		win.Update(func() {
			a.updateBusy = false
			if err != nil {
				logger.Printf("update: %v", err)
				a.updateStatus = locales.UpdateFailed(err.Error())
			} else if pending.Version != "" {
				a.updatePending = pending.Version
				a.updateStatus = locales.UpdateReady(pending.Version)
			} else if manual {
				a.updateStatus = locales.UpdateLatest()
			} else {
				a.updateStatus = ""
			}
		})
	}()
}

func (a *app) updateBanner(c *ui.Context) {
	if a.updatePending == "" {
		return
	}
	t := c.Theme()
	busy := a.moduleBusy || a.installingEditor()
	ui.Row(c).Padding(10, 24).Gap(12).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		status := a.updateStatus
		if busy {
			status = locales.UpdateInstallWait()
		}
		ui.Text(c, status).Grow(1).FontSize(12)
		ui.PrimaryButton(c, locales.UpdateRestart()).Disabled(busy).OnClick(a.applyUpdateNow)
	})
}

func (a *app) applyUpdateNow() {
	if a.installingEditor() {
		a.alert(locales.DownloadInstallInProgress())
		return
	}
	if a.moduleBusy {
		a.manageOpen, a.manageModules = true, true
		return
	}
	if _, err := readPendingUpdate(); err != nil {
		a.updateStatus = locales.UpdateFailed(err.Error())
		return
	}
	if err := startUpdateHelper(os.Getpid()); err != nil {
		a.updateStatus = locales.UpdateFailed(err.Error())
		return
	}
	mygo.App.Quit()
}
