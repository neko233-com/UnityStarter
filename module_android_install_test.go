package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func androidArchiveFixtures(t *testing.T) (androidInstallPlan, []string) {
	t.Helper()
	type fixture struct {
		id, dest, from, to string
		files              map[string]string
	}
	fixtures := []fixture{
		{"sdk", "SDK", "", "", map[string]string{}},
		{"ndk", "NDK", "NDK/android-ndk-r27c", "NDK", map[string]string{"android-ndk-r27c/source.properties": "ndk"}},
		{"cmake", "SDK/cmake/cmake", "SDK/cmake/cmake", "SDK/cmake/3.22.1", map[string]string{"bin/cmake.exe": "cmake"}},
		{"build-tools", "SDK/build-tools", "SDK/build-tools/android-16", "SDK/build-tools/36.0.0", map[string]string{"android-16/aapt.exe": "aapt"}},
		{"platform-tools", "SDK", "", "", map[string]string{"platform-tools/adb.exe": "adb"}},
		{"platform", "SDK/platforms", "", "", map[string]string{"android-34/android.jar": "jar"}},
		{"cmdline", "SDK/cmdline-tools", "SDK/cmdline-tools/cmdline-tools", "SDK/cmdline-tools/21.0", map[string]string{"cmdline-tools/bin/sdkmanager.bat": "sdkmanager"}},
	}
	plan := androidInstallPlan{License: "Terms fixture"}
	var archives []string
	for _, f := range fixtures {
		archive := makeModuleZip(t, f.files)
		archives = append(archives, archive)
		plan.Packages = append(plan.Packages, androidPackage{Module: editorModule{ID: f.id}, Destination: f.dest, RenameFrom: f.from, RenameTo: f.to, Integrity: archiveIntegrity(t, archive)})
	}
	return plan, archives
}

func TestAndroidDownloadRunsPackagesConcurrently(t *testing.T) {
	body := []byte("verified archive fixture")
	sum := sha256.Sum256(body)
	var started atomic.Int32
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusOK)
			return
		}
		if started.Add(1) == 3 {
			close(ready)
		}
		select {
		case <-ready:
			_, _ = w.Write(body)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	var plan androidInstallPlan
	for i := 0; i < 3; i++ {
		plan.Packages = append(plan.Packages, androidPackage{Module: editorModule{URL: server.URL, DownloadSize: int64(len(body))}, Integrity: "sha256-" + base64.StdEncoding.EncodeToString(sum[:]), ExpectedSize: int64(len(body))})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	paths, err := downloadAndroidPackages(ctx, plan, t.TempDir(), func(int64, int64) {})
	if err != nil || len(paths) != 3 || started.Load() != 3 {
		t.Fatalf("parallel downloads: %v %v started=%d", paths, err, started.Load())
	}
	for _, path := range paths {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(body) {
			t.Fatalf("download %s: %q %v", path, got, err)
		}
	}
}

func TestAndroidInstallStagesAndCommitsAllArchives(t *testing.T) {
	root := fakeEditorRoot(t)
	plan, archives := androidArchiveFixtures(t)
	if err := installAndroidArchives(root, plan, archives); err != nil {
		t.Fatal(err)
	}
	player := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
	if !androidSDKFilesPresent(player) {
		t.Fatal("expected SDK and NDK files missing")
	}
	license, err := os.ReadFile(filepath.Join(player, "SDK", "licenses", "android-sdk-license"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum([]byte(plan.License))
	if !strings.Contains(string(license), fmt.Sprintf("%x", sum)) {
		t.Fatalf("accepted license hash missing: %q", license)
	}
	if err := installAndroidArchives(root, plan, archives); err == nil {
		t.Fatal("existing SDK was overwritten")
	}
	if data, err := os.ReadFile(filepath.Join(player, "NDK", "source.properties")); err != nil || string(data) != "ndk" {
		t.Fatalf("existing NDK changed: %q %v", data, err)
	}
}

func TestAndroidInstallDoesNotPublishPartialOnFailure(t *testing.T) {
	root := fakeEditorRoot(t)
	plan, archives := androidArchiveFixtures(t)
	plan.Packages[3].Integrity = plan.Packages[2].Integrity
	if err := installAndroidArchives(root, plan, archives); err == nil {
		t.Fatal("bad archive integrity accepted")
	}
	player := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
	if entries, err := os.ReadDir(player); err != nil || len(entries) != 0 {
		t.Fatalf("partial install remains: %v %v", entries, err)
	}
}

func TestAndroidInstallRejectsMissingLicenseTextAndUnsafeDestination(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*androidInstallPlan)
	}{
		{"missing license", func(plan *androidInstallPlan) { plan.License = "" }},
		{"unsafe destination", func(plan *androidInstallPlan) { plan.Packages[0].Destination = "../../outside" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fakeEditorRoot(t)
			plan, archives := androidArchiveFixtures(t)
			test.mutate(&plan)
			if err := installAndroidArchives(root, plan, archives); err == nil {
				t.Fatal("invalid plan accepted")
			}
			player := filepath.Join(root, "Editor", "Data", "PlaybackEngines", "AndroidPlayer")
			if entries, err := os.ReadDir(player); err != nil || len(entries) != 0 {
				t.Fatalf("partial install remains: %v %v", entries, err)
			}
		})
	}
}
