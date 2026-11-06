package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const androidLicenseURL = "https://dl.google.com/dl/android/repository/repository2-1.xml"

var androidRepositoryURL = "https://dl.google.com/android/repository/repository2-3.xml"
var androidLicenseRepositoryURL = androidLicenseURL

type androidRepository struct {
	Licenses []struct {
		ID   string `xml:"id,attr"`
		Text string `xml:",chardata"`
	} `xml:"license"`
	Packages []struct {
		License struct {
			Ref string `xml:"ref,attr"`
		} `xml:"uses-license"`
		Archives []struct {
			Host     string `xml:"host-os"`
			Complete struct {
				URL      string `xml:"url"`
				Size     int64  `xml:"size"`
				Checksum struct {
					Type  string `xml:"type,attr"`
					Value string `xml:",chardata"`
				} `xml:"checksum"`
			} `xml:"complete"`
		} `xml:"archives>archive"`
	} `xml:"remotePackage"`
}

type androidPackage struct {
	Module       editorModule
	Integrity    string
	Destination  string
	RenameFrom   string
	RenameTo     string
	ExpectedSize int64
}

type androidInstallPlan struct {
	Module          editorModule
	Packages        []androidPackage
	License         string
	CachedBytes     int64
	RecoveryPending bool
	LegacyTuanjie   bool
}

func androidSDKCandidate(e editorInstall, m editorModule) bool {
	return parentModuleSelected(e.Modules, "android") && ((e.Kind == kindUnity && androidSDKMetadata(m)) || (e.Kind == kindTuanjie && legacyAndroidSDKMetadata(m)))
}

func androidSDKMetadata(m editorModule) bool {
	if m.ID != "android-sdk-ndk-tools" || m.Parent != "android" || m.EULA != androidLicenseURL || m.EULA2 != "" || m.Command != "" || m.Destination != "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/SDK" {
		return false
	}
	_, ok := trustedUnityArchiveIntegrity(m.URL)
	return ok
}

func trustedUnityArchiveIntegrity(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "download.unity3d.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	match := officialArchiveDigest.FindStringSubmatch(u.Path)
	if len(match) != 2 {
		return "", false
	}
	b, err := hex.DecodeString(match[1])
	if err != nil || len(b) != 32 {
		return "", false
	}
	return "sha256-" + base64.StdEncoding.EncodeToString(b), true
}

func androidRelativePath(value string) (string, error) {
	const prefix = "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/"
	if !strings.HasPrefix(value, prefix) {
		return "", fmt.Errorf("invalid Android module destination")
	}
	rel := strings.TrimPrefix(value, prefix)
	if rel == "" || path.Clean(rel) != rel || strings.ContainsAny(rel, `\:`) {
		return "", fmt.Errorf("invalid Android module destination")
	}
	parts := strings.Split(rel, "/")
	if parts[0] != "SDK" && parts[0] != "NDK" {
		return "", fmt.Errorf("invalid Android module destination")
	}
	for _, part := range parts {
		if !safeArchiveComponent(part) {
			return "", fmt.Errorf("invalid Android module destination")
		}
	}
	return rel, nil
}

func fetchAndroidRepository(ctx context.Context, endpoint string) (androidRepository, error) {
	var repo androidRepository
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return repo, err
	}
	origin := req.URL
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 || next.URL.Scheme != origin.Scheme || next.URL.Host != origin.Host {
			return fmt.Errorf("untrusted Android repository redirect")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return repo, fmt.Errorf("Android repository unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return repo, fmt.Errorf("Android repository: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return repo, err
	}
	if len(data) > 8<<20 {
		return repo, fmt.Errorf("Android repository too large")
	}
	if err := xml.Unmarshal(data, &repo); err != nil {
		return repo, err
	}
	return repo, nil
}

func androidLicense(repo androidRepository) string {
	for _, license := range repo.Licenses {
		if license.ID == "android-sdk-license" {
			return license.Text
		}
	}
	return ""
}

func prepareAndroidPlan(ctx context.Context, e editorInstall, m editorModule) (androidInstallPlan, error) {
	var plan androidInstallPlan
	if !androidSDKCandidate(e, m) {
		return plan, fmt.Errorf("unsupported Android SDK metadata")
	}
	google, err := fetchAndroidRepository(ctx, androidRepositoryURL)
	if err != nil {
		return plan, err
	}
	licenseRepo, err := fetchAndroidRepository(ctx, androidLicenseRepositoryURL)
	if err != nil {
		return plan, err
	}
	license := androidLicense(licenseRepo)
	if strings.TrimSpace(license) == "" || license != androidLicense(google) {
		return plan, fmt.Errorf("Android license terms differ or are unavailable")
	}
	if e.Kind == kindTuanjie {
		return prepareLegacyTuanjieAndroidPlan(e, m, license)
	}
	baseIntegrity, _ := trustedUnityArchiveIntegrity(m.URL)
	plan.Module, plan.License = m, license
	if m.DownloadSize <= 0 || m.DownloadSize > 8<<30 {
		return androidInstallPlan{}, fmt.Errorf("invalid Android archive size")
	}
	plan.Packages = append(plan.Packages, androidPackage{Module: m, Integrity: baseIntegrity, Destination: "SDK", ExpectedSize: m.DownloadSize})
	seen := map[string]bool{}
	families := map[string]bool{}
	for _, child := range e.Modules {
		if child.Parent != m.ID || !child.Required {
			continue
		}
		if seen[child.ID] || !moduleIDPattern.MatchString(child.ID) || child.EULA != "" || child.EULA2 != "" || child.Command != "" {
			return androidInstallPlan{}, fmt.Errorf("invalid Android dependency")
		}
		seen[child.ID] = true
		for _, family := range []string{"android-ndk-", "cmake-", "android-sdk-build-tools-", "android-sdk-platform-tools-", "android-sdk-platforms-", "android-sdk-command-line-tools-"} {
			if strings.HasPrefix(child.ID, family) {
				families[family] = true
			}
		}
		u, err := url.Parse(child.URL)
		if err != nil || u.Scheme != "https" || u.Host != "dl.google.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/android/repository/") || path.Base(u.Path) != strings.TrimPrefix(u.Path, "/android/repository/") || !strings.HasSuffix(u.Path, ".zip") {
			return androidInstallPlan{}, fmt.Errorf("untrusted Android dependency URL")
		}
		dest, err := androidRelativePath(child.Destination)
		if err != nil {
			return androidInstallPlan{}, err
		}
		from, to := "", ""
		if child.RenameFrom != "" || child.RenameTo != "" {
			from, err = androidRelativePath(child.RenameFrom)
			if err != nil {
				return androidInstallPlan{}, err
			}
			to, err = androidRelativePath(child.RenameTo)
			if err != nil {
				return androidInstallPlan{}, err
			}
			if strings.Split(from, "/")[0] != strings.Split(to, "/")[0] {
				return androidInstallPlan{}, fmt.Errorf("invalid Android archive rename")
			}
		}
		var integrity string
		var size int64
		matches := 0
		for _, pkg := range google.Packages {
			for _, archive := range pkg.Archives {
				if (archive.Host != "" && archive.Host != "windows") || archive.Complete.URL != path.Base(u.Path) {
					continue
				}
				matches++
				sum, sumErr := hex.DecodeString(strings.TrimSpace(archive.Complete.Checksum.Value))
				if pkg.License.Ref != "android-sdk-license" || archive.Complete.Checksum.Type != "sha1" || sumErr != nil || len(sum) != 20 || archive.Complete.Size <= 0 || (child.DownloadSize > 0 && archive.Complete.Size != child.DownloadSize) {
					return androidInstallPlan{}, fmt.Errorf("invalid Android repository checksum or license")
				}
				integrity = "sha1-" + base64.StdEncoding.EncodeToString(sum)
				size = archive.Complete.Size
			}
		}
		if matches != 1 {
			return androidInstallPlan{}, fmt.Errorf("Android dependency %s missing or ambiguous in official repository (%d matches)", child.ID, matches)
		}
		if size > 8<<30 {
			return androidInstallPlan{}, fmt.Errorf("Android dependency is too large")
		}
		if child.Integrity != "" {
			h, _, err := integrityHasher(child.Integrity)
			if err != nil || h == nil || !strings.HasPrefix(child.Integrity, "sha384-") {
				return androidInstallPlan{}, fmt.Errorf("unsupported Android dependency integrity")
			}
			integrity = child.Integrity
		}
		plan.Packages = append(plan.Packages, androidPackage{Module: child, Integrity: integrity, Destination: dest, RenameFrom: from, RenameTo: to, ExpectedSize: size})
		if len(plan.Packages) > 64 {
			return androidInstallPlan{}, fmt.Errorf("too many Android dependencies")
		}
	}
	if len(families) != 6 {
		return androidInstallPlan{}, fmt.Errorf("Android dependency list is incomplete")
	}
	return plan, nil
}
