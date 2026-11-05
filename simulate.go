package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// simulatedMarker marks an editor folder this app made as a stand-in, so that
// the whole install flow can be tried without downloading gigabytes.
const simulatedMarker = ".unitystarter-simulated"

// simulateEditorInstall writes the folders and files a real installer would,
// with an executable that only exists for the app to find.
func simulateEditorInstall(engine, version, root string) (string, error) {
	dir := filepath.Join(root, version)
	editorDir := filepath.Join(dir, "Editor")
	if err := os.MkdirAll(editorDir, 0o755); err != nil {
		return "", err
	}
	exeName := "Unity.exe"
	if engine == kindTuanjie {
		exeName = "Tuanjie.exe"
	}
	exe := filepath.Join(editorDir, exeName)
	if err := os.WriteFile(exe, []byte("simulated editor\n"), 0o644); err != nil {
		return "", err
	}
	marker := fmt.Sprintf("engine=%s\nversion=%s\n", engine, version)
	if err := os.WriteFile(filepath.Join(dir, simulatedMarker), []byte(marker), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "modules.json"), []byte("[]"), 0o644); err != nil {
		return "", err
	}
	logger.Printf("simulate: %s %s at %s", engine, version, dir)
	return dir, nil
}

// isSimulatedEditor reports whether an editor folder is a stand-in.
func isSimulatedEditor(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, simulatedMarker))
	return err == nil
}

// removeSimulatedEditor deletes a stand-in the user no longer wants.
func removeSimulatedEditor(dir string) error {
	if !isSimulatedEditor(dir) {
		return fmt.Errorf("%s is not simulated", dir)
	}
	return os.RemoveAll(dir)
}
