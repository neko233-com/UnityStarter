package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const googleArchiveBase = "https://dl.google.com/android/repository/"
const androidPlayerPlaceholder = "{UNITY_PATH}/Editor/Data/PlaybackEngines/AndroidPlayer/"

// Fixed baselines observed from Google's HTTPS downloads on 2026-11-06.
// Some archives no longer appear in Google's current repository metadata;
// these hashes detect subsequent changes but are not Google-signed checksums.
type legacyAndroidArchive struct {
	id, filename, destination, renameFrom, renameTo, sha256 string
	size                                                    int64
}

var legacyTuanjieAndroidArchives = []legacyAndroidArchive{
	{"android-sdk-ndk-tools", "sdk-tools-windows-4333796.zip", "SDK", "", "", "7e81d69c303e47a4f0e748a6352d85cd0c8fd90a5a95ae4e076b5e5f960d3c7a", 156136858},
	{"android-sdk-platform-tools", "platform-tools_r32.0.0-windows.zip", "SDK", "", "", "41f4c7512b32cbb3f8c624c20b56326abb692a6f169b03b4b63b6c5a6fdbb08c", 12033552},
	{"android-sdk-build-tools", "build-tools_r34-windows.zip", "SDK/build-tools", "SDK/build-tools/android-14", "SDK/build-tools/34.0.0", "9be665ad74ef22bf0e489b37db465074484df79b95f26d759945d5c25a47b326", 58253258},
	{"android-sdk-platforms-33", "platform-33_r02.zip", "SDK/platforms", "SDK/platforms/android-13", "SDK/platforms/android-33", "f851b13fe89f8510a1250df5e8593e86176b2428f4f3cbe0e304a85818c07bc8", 67334130},
	{"android-sdk-platforms-34", "platform-34-ext7_r02.zip", "SDK/platforms", "", "", "5323311cc3e4ad614f0b8053c72b651726f3422448cedd39e48f00737cda8ad0", 63180079},
	{"android-sdk-platforms-35", "platform-35_r01.zip", "SDK/platforms", "", "", "ec20a0a65704e1b2554f8e3eebf588ef260569673c848c01aff2e29c28734cf4", 64281654},
	{"android-ndk", "android-ndk-r23b-windows.zip", "NDK", "NDK/android-ndk-r23b", "NDK", "36270f8b2cfdd940f410bd8ffe6ce86ffaa8e87ff1c4fd4a8be5130268357778", 788638042},
	{"android-sdk-command-line-tools", "commandlinetools-win-8092744_latest.zip", "SDK/cmdline-tools", "SDK/cmdline-tools/cmdline-tools", "SDK/cmdline-tools/6.0", "5de99ed67cb2e30fe443baf8b282d1b0b6247d0c25c6d888a7e8657b3b35c281", 119629490},
}

func legacyAndroidSDKMetadata(m editorModule) bool {
	return m.ID == "android-sdk-ndk-tools" && m.Parent == "android" && m.Sync == "Android Build Support" && m.URL == googleArchiveBase+legacyTuanjieAndroidArchives[0].filename && m.Destination == androidPlayerPlaceholder+"SDK" && m.EULA == androidLicenseURL && m.EULA2 == "" && m.Command == "" && m.Checksum == "" && m.Integrity == "" && m.RenameFrom == "" && m.RenameTo == ""
}

func prepareLegacyTuanjieAndroidPlan(e editorInstall, m editorModule, license string) (androidInstallPlan, error) {
	if e.Kind != kindTuanjie || !legacyAndroidSDKMetadata(m) || !parentModuleSelected(e.Modules, "android") || strings.TrimSpace(license) == "" || m.DependencyFingerprint == "" {
		return androidInstallPlan{}, fmt.Errorf("unsupported Tuanjie Android metadata")
	}
	byID := make(map[string]editorModule)
	for _, child := range e.Modules {
		if child.Sync != m.ID {
			continue
		}
		if _, exists := byID[child.ID]; exists {
			return androidInstallPlan{}, fmt.Errorf("duplicate Tuanjie Android dependency")
		}
		byID[child.ID] = child
	}
	if len(byID) != len(legacyTuanjieAndroidArchives)-1 {
		return androidInstallPlan{}, fmt.Errorf("Tuanjie Android dependency list is incomplete")
	}
	plan := androidInstallPlan{Module: m, License: license, LegacyTuanjie: true}
	for i, spec := range legacyTuanjieAndroidArchives {
		child := m
		if i > 0 {
			var ok bool
			child, ok = byID[spec.id]
			if !ok {
				return androidInstallPlan{}, fmt.Errorf("missing Tuanjie Android dependency %s", spec.id)
			}
		}
		from, to := "", ""
		if spec.renameFrom != "" {
			from = androidPlayerPlaceholder + spec.renameFrom
		}
		if spec.renameTo != "" {
			to = androidPlayerPlaceholder + spec.renameTo
		}
		if child.ID != spec.id || child.URL != googleArchiveBase+spec.filename || child.Destination != androidPlayerPlaceholder+spec.destination || child.RenameFrom != from || child.RenameTo != to || child.Command != "" || child.Checksum != "" || child.Integrity != "" || child.EULA2 != "" || (i > 0 && (child.Parent != "" || child.EULA != "")) {
			return androidInstallPlan{}, fmt.Errorf("unexpected Tuanjie Android dependency metadata: %s", spec.id)
		}
		digest, err := hex.DecodeString(spec.sha256)
		if err != nil || len(digest) != 32 {
			return androidInstallPlan{}, fmt.Errorf("invalid Android archive baseline")
		}
		plan.Packages = append(plan.Packages, androidPackage{Module: child, Integrity: "sha256-" + base64.StdEncoding.EncodeToString(digest), Destination: spec.destination, RenameFrom: spec.renameFrom, RenameTo: spec.renameTo, ExpectedSize: spec.size})
	}
	return plan, nil
}
