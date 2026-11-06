package main

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
)

const openHarmonyNodeModule = "openharmony-nodejs"
const openHarmonyNodeFolder = "node-v18.14.1-win-x64"

// The Tuanjie catalog currently points to this exact Tencent mirror archive.
// The digest is published by Node.js in its v18.14.1 release checksums.
func openHarmonyNodeArchive(m editorModule) (string, bool) {
	if m.ID != openHarmonyNodeModule || m.Parent != "openharmony" ||
		m.URL != "https://mirrors.cloud.tencent.com/nodejs-release/v18.14.1/node-v18.14.1-win-x64.zip" ||
		m.Destination != "{UNITY_PATH}/Editor/Data/PlaybackEngines/OpenHarmonyPlayer/Nodejs" ||
		m.Command != "" || m.EULA != "" || m.EULA2 != "" || m.Checksum != "" || m.Integrity != "" || m.RenameFrom != "" || m.RenameTo != "" {
		return "", false
	}
	sum, _ := hex.DecodeString("b0ec709961ab9b2e46d616d33f26d0ff1548b02045c7650f018e809e7a6cbbda")
	return "sha256-" + base64.StdEncoding.EncodeToString(sum), true
}

func installOpenHarmonyNode(root, archive, integrity string) error {
	return installVerifiedToolArchive(root, archive, "OpenHarmonyPlayer", "Nodejs", filepath.Join(openHarmonyNodeFolder, "node.exe"), integrity)
}

func openHarmonyNodeFilesPresent(dir string) bool {
	for _, rel := range []string{"node.exe", "npm.cmd", "node_modules/npm/bin/npm-cli.js"} {
		info, err := os.Stat(filepath.Join(dir, openHarmonyNodeFolder, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}
