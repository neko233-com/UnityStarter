package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"unitystarter/locales"
)

func TestParseUnityLicenseStatus(t *testing.T) {
	tests := []struct {
		name, json, want string
		valid            bool
	}{
		{"active", `{"success":true,"command":"license status","data":{"active":true,"machineId":"private"},"errors":[]}`, "active", true},
		{"inactive", `{"success":false,"command":"license status","data":{"active":false},"errors":[{"code":"LICENSE_NOT_ACTIVE"}]}`, "inactive", true},
		{"service failure", `{"success":false,"command":"license status","data":{"active":false},"errors":[{"code":"SERVICE_ERROR"}]}`, "", false},
		{"contradictory", `{"success":true,"command":"license status","data":{"active":true},"errors":[{"code":"SERVICE_ERROR"}]}`, "", false},
		{"missing status", `{"success":true,"command":"license status","data":{},"errors":[]}`, "", false},
		{"wrong command", `{"success":true,"command":"license","data":{"active":true}}`, "", false},
		{"invalid json", `{`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUnityLicenseStatus([]byte(tt.json))
			if (err == nil) != tt.valid || got != tt.want {
				t.Fatalf("state=%q error=%v", got, err)
			}
		})
	}
}

func TestLicenseStatusInNativePage(t *testing.T) {
	a := fixture()
	a.page = pageLicense
	a.licenseCheckState = "inactive"
	tt := ui.NewTester(a.view, 900, 560)
	if !tt.HasText(locales.LicenseCheck()) || !tt.HasText(locales.LicenseInactive()) {
		t.Fatalf("license status controls missing: %q", tt.Texts())
	}
	if path := os.Getenv("LICENSE_STATUS_SNAPSHOT"); path != "" {
		tt.Frame()
		if err := writePNG(path, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnityLicenseBannerUsesExplicitCheckAndEngine(t *testing.T) {
	a := fixture()
	a.activeEngine, a.page = kindUnity, pageProjects
	tt := ui.NewTester(a.view, 1180, 760)
	if tt.HasText(locales.LicenseInactiveBanner()) {
		t.Fatal("unknown license status should not be presented as inactive")
	}
	a.licenseCheckState = "inactive"
	tt.Frame()
	if !tt.HasText(locales.LicenseInactiveBanner()) {
		t.Fatalf("missing inactive license banner: %q", tt.Texts())
	}
	a.activeEngine, a.page = kindTuanjie, pageTuanjieProjects
	tt.Frame()
	if tt.HasText(locales.LicenseInactiveBanner()) {
		t.Fatal("Unity license status leaked into the Tuanjie workspace")
	}
	a.activeEngine, a.page = kindUnity, pageProjects
	tt.Frame()
	if err := tt.Click(locales.LicenseView()); err != nil {
		t.Fatal(err)
	}
	if a.page != pageLicense || tt.HasText(locales.LicenseInactiveBanner()) {
		t.Fatalf("license page did not open: %q", tt.Texts())
	}
}

func TestRealUnityLicenseStatus(t *testing.T) {
	want := os.Getenv("UNITYSTARTER_EXPECT_LICENSE_STATE")
	if want == "" {
		t.Skip("set UNITYSTARTER_EXPECT_LICENSE_STATE for a local Unity CLI check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	got, err := unityLicenseStatus(ctx)
	if err != nil || got != want {
		t.Fatalf("state=%q error=%v", got, err)
	}
}
