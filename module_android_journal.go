package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const androidJournalName = "install-journal.json"
const androidStageMarker = ".unitystarter-transaction"

type androidJournal struct {
	Version int    `json:"version"`
	Key     string `json:"key"`
	Stage   string `json:"stage"`
	SDKHash string `json:"sdk_sha256"`
	NDKHash string `json:"ndk_sha256"`
}

// Hash names, kinds, lengths and contents so recovery never moves or removes
// a directory that somebody changed after the interrupted installation.
func androidTreeHash(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("unsafe Android recovery entry")
		}
		label := filepath.ToSlash(rel)
		var num [8]byte
		binary.BigEndian.PutUint64(num[:], uint64(len(label)))
		_, _ = h.Write(num[:])
		_, _ = io.WriteString(h, label)
		if info.IsDir() {
			_, _ = h.Write([]byte{0})
			return nil
		}
		_, _ = h.Write([]byte{1})
		binary.BigEndian.PutUint64(num[:], uint64(info.Size()))
		_, _ = h.Write(num[:])
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			file.Close()
			return fmt.Errorf("Android recovery entry changed")
		}
		n, copyErr := io.Copy(h, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != info.Size() {
			return fmt.Errorf("Android recovery entry changed")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func prepareAndroidJournal(cache *androidCache, stage string) error {
	if cache == nil {
		return nil
	}
	parent := filepath.Dir(stage)
	rel, err := filepath.Rel(parent, stage)
	if err != nil || filepath.Dir(rel) != "." || !strings.HasPrefix(rel, ".unitystarter-android-") {
		return fmt.Errorf("invalid Android staging directory")
	}
	marker := filepath.Join(stage, androidStageMarker)
	if err := os.WriteFile(marker, []byte(cache.key+"\n"), 0o600); err != nil {
		return err
	}
	sdk, err := androidTreeHash(filepath.Join(stage, "SDK"))
	if err != nil {
		return err
	}
	ndk, err := androidTreeHash(filepath.Join(stage, "NDK"))
	if err != nil {
		return err
	}
	data, err := json.Marshal(androidJournal{Version: 1, Key: cache.key, Stage: rel, SDKHash: sdk, NDKHash: ndk})
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(cache.dir, androidJournalName), data)
}

func readAndroidJournal(cache *androidCache) (androidJournal, bool, error) {
	var j androidJournal
	path := filepath.Join(cache.dir, androidJournalName)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return j, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 4096 {
		return j, false, fmt.Errorf("Android recovery journal is unsafe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return j, false, err
	}
	if json.Unmarshal(data, &j) != nil || j.Version != 1 || j.Key != cache.key || filepath.Base(j.Stage) != j.Stage || !strings.HasPrefix(j.Stage, ".unitystarter-android-") || !validSHA256(j.SDKHash) || !validSHA256(j.NDKHash) {
		return j, false, fmt.Errorf("Android recovery journal is invalid")
	}
	return j, true, nil
}

func androidJournalPending(base, root string, plan androidInstallPlan) (bool, error) {
	_, dir, _, err := androidCacheLocation(base, root, plan)
	if err != nil {
		return false, err
	}
	key, _, err := androidCacheIdentity(root, plan)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("Android cache directory is unsafe")
	}
	marker, err := readAndroidCacheMarker(dir)
	if err != nil || string(marker) != key+"\n" {
		return false, fmt.Errorf("Android cache ownership cannot be verified")
	}
	_, pending, err := readAndroidJournal(&androidCache{dir: dir, key: key})
	return pending, err
}

// The caller holds the editor-specific cache lock and has received explicit
// license consent. No existing directory is replaced unless it is empty.
func recoverAndroidJournal(cache *androidCache, root string) (bool, error) {
	j, pending, err := readAndroidJournal(cache)
	if err != nil || !pending {
		return false, err
	}
	parent, err := androidPlayerDir(root)
	if err != nil {
		return false, err
	}
	stage := filepath.Join(parent, j.Stage)
	info, err := os.Lstat(stage)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("Android recovery staging directory is missing or unsafe")
	}
	markerPath := filepath.Join(stage, androidStageMarker)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 || markerInfo.Size() > 128 {
		return false, fmt.Errorf("Android recovery marker is unsafe")
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil || string(marker) != cache.key+"\n" {
		return false, fmt.Errorf("Android recovery marker is invalid")
	}
	type candidate struct {
		name, source, expected string
		move                   bool
		removeEmpty            bool
	}
	var candidates []candidate
	for _, item := range []struct{ name, hash string }{{"SDK", j.SDKHash}, {"NDK", j.NDKHash}} {
		staged := filepath.Join(stage, item.name)
		dest := filepath.Join(parent, item.name)
		stagedInfo, stageErr := os.Lstat(staged)
		if stageErr != nil && !os.IsNotExist(stageErr) {
			return false, stageErr
		}
		if stageErr == nil && (!stagedInfo.IsDir() || stagedInfo.Mode()&os.ModeSymlink != 0) {
			return false, fmt.Errorf("Android recovery source is unsafe")
		}
		empty, destErr := emptyAndroidDestination(parent, item.name)
		if stageErr == nil {
			if destErr != nil {
				return false, destErr
			}
			candidates = append(candidates, candidate{item.name, staged, item.hash, true, empty})
		} else {
			if destErr == nil {
				return false, fmt.Errorf("Android recovery destination is missing")
			}
			// A nonempty destination can be ours only when its exact tree hash matches.
			destInfo, err := os.Lstat(dest)
			if err != nil || !destInfo.IsDir() || destInfo.Mode()&os.ModeSymlink != 0 {
				return false, fmt.Errorf("Android recovery destination is unsafe")
			}
			candidates = append(candidates, candidate{item.name, dest, item.hash, false, false})
		}
	}
	for _, c := range candidates {
		actual, err := androidTreeHash(c.source)
		if err != nil {
			return false, err
		}
		if actual != c.expected {
			return false, fmt.Errorf("Android %s files changed after interruption; existing files were preserved", c.name)
		}
	}
	for _, c := range candidates {
		if !c.move {
			continue
		}
		dest := filepath.Join(parent, c.name)
		if c.removeEmpty {
			if err := os.Remove(dest); err != nil {
				return false, err
			}
		}
		if err := os.Rename(c.source, dest); err != nil {
			if c.removeEmpty {
				_ = os.Mkdir(dest, 0o755)
			}
			return false, err
		}
	}
	if !androidSDKFilesPresent(parent) {
		return false, fmt.Errorf("recovered Android files are incomplete")
	}
	return true, nil
}

// Called only after installation state has been persisted. The transaction
// remains recoverable until then, including after another process crash.
func finalizeAndroidJournal(cache *androidCache, root string) error {
	j, pending, err := readAndroidJournal(cache)
	if err != nil || !pending {
		return err
	}
	parent, err := androidPlayerDir(root)
	if err != nil {
		return err
	}
	stage := filepath.Join(parent, j.Stage)
	info, err := os.Lstat(stage)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Android recovery stage is missing or unsafe")
	}
	markerPath := filepath.Join(stage, androidStageMarker)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 || markerInfo.Size() > 128 {
		return fmt.Errorf("Android recovery marker is unsafe")
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil || string(marker) != cache.key+"\n" {
		return fmt.Errorf("Android recovery marker is invalid")
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if len(entries) != 1 || entries[0].Name() != androidStageMarker {
		return fmt.Errorf("Android recovery stage contains unowned files")
	}
	if err := os.Remove(markerPath); err != nil {
		return err
	}
	if err := os.Remove(stage); err != nil {
		_ = os.WriteFile(markerPath, []byte(cache.key+"\n"), 0o600)
		return err
	}
	return os.Remove(filepath.Join(cache.dir, androidJournalName))
}
