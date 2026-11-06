package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func androidFixture() (editorInstall, editorModule) {
	root := editorModule{ID: "android-sdk-ndk-tools", Parent: "android", URL: "https://download.unity3d.com/1_" + strings.Repeat("a", 64) + ".zip", Destination: "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/SDK", EULA: androidLicenseURL, DownloadSize: 166}
	modules := []editorModule{{ID: "android", Selected: true}, root}
	for _, id := range []string{"android-ndk-r27c", "cmake-3.22.1", "android-sdk-build-tools-36.0.0", "android-sdk-platform-tools-37.0.1", "android-sdk-platforms-34", "android-sdk-command-line-tools-21.0"} {
		modules = append(modules, editorModule{ID: id, Parent: root.ID, Required: true, URL: "https://dl.google.com/android/repository/" + id + ".zip", Destination: "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/SDK", DownloadSize: 42})
	}
	return editorInstall{Kind: kindUnity, Modules: modules}, root
}

func TestAndroidPlanUsesOfficialDependencyChecksumsAndLicense(t *testing.T) {
	e, root := androidFixture()
	var b strings.Builder
	b.WriteString(`<sdk-repository><license id="android-sdk-license">Terms fixture</license>`)
	for _, child := range e.Modules[2:] {
		fmt.Fprintf(&b, `<remotePackage path="%s"><uses-license ref="android-sdk-license"/><archives><archive><complete><size>42</size><checksum type="sha1">ac5f7762764b1f15341094e148ad4f847d050c38</checksum><url>%s.zip</url></complete><host-os>windows</host-os></archive></archives></remotePackage>`, child.ID, child.ID)
	}
	b.WriteString(`</sdk-repository>`)
	repository := b.String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(repository)) }))
	defer server.Close()
	oldRepo, oldLicense := androidRepositoryURL, androidLicenseRepositoryURL
	androidRepositoryURL, androidLicenseRepositoryURL = server.URL, server.URL
	defer func() { androidRepositoryURL, androidLicenseRepositoryURL = oldRepo, oldLicense }()
	plan, err := prepareAndroidPlan(context.Background(), e, root)
	if err != nil || len(plan.Packages) != 7 || plan.License != "Terms fixture" || !strings.HasPrefix(plan.Packages[1].Integrity, "sha1-") {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	e.Modules[2].URL = "https://evil.example/android-ndk-r27c-windows.zip"
	if _, err := prepareAndroidPlan(context.Background(), e, root); err == nil {
		t.Fatal("untrusted dependency accepted")
	}
}

func TestLiveUnity7AndroidPlan(t *testing.T) {
	if os.Getenv("UNITYSTARTER_LIVE_CATALOG") != "1" {
		t.Skip("set UNITYSTARTER_LIVE_CATALOG=1 to validate official metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	modules, err := fetchUnityModules(ctx, "7000.0.0a16")
	if err != nil {
		t.Fatal(err)
	}
	var sdk editorModule
	for i := range modules {
		if modules[i].ID == "android" {
			modules[i].Selected = true
		}
		if modules[i].ID == "android-sdk-ndk-tools" {
			sdk = modules[i]
		}
	}
	plan, err := prepareAndroidPlan(ctx, editorInstall{Kind: kindUnity, Modules: modules}, sdk)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Packages) != 9 || len(plan.License) < 1000 {
		t.Fatalf("incomplete plan: packages=%d license=%d", len(plan.Packages), len(plan.License))
	}
	t.Logf("official Unity 7 Android plan: %d packages, license %d characters", len(plan.Packages), len(plan.License))
}

func TestAndroidDependencyFingerprintTracksMetadata(t *testing.T) {
	e, _ := androidFixture()
	annotateSDKDependencies(e.Modules)
	initial := moduleFingerprint(e.Modules[1])
	e.Modules[2].Selected = true
	annotateSDKDependencies(e.Modules)
	if got := moduleFingerprint(e.Modules[1]); got != initial {
		t.Fatal("installation status changed metadata fingerprint")
	}
	e.Modules[2].URL += "-changed"
	annotateSDKDependencies(e.Modules)
	if got := moduleFingerprint(e.Modules[1]); got == initial {
		t.Fatal("changed dependency did not invalidate installed state")
	}
}
