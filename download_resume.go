package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type resumeState struct {
	Version    int     `json:"version"`
	SourceHash string  `json:"source_hash"`
	Integrity  string  `json:"integrity"`
	Validator  string  `json:"validator"`
	Size       int64   `json:"size"`
	Completed  []int64 `json:"completed"`
}

func downloadIdentity(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func resumePaths(to string) (part, statePath, lockKey string, err error) {
	if to == "" || filepath.Clean(to) == "." || filepath.Base(filepath.Clean(to)) == string(filepath.Separator) {
		return "", "", "", fmt.Errorf("invalid download destination")
	}
	absolute, err := filepath.Abs(to)
	if err != nil {
		return "", "", "", err
	}
	lockKey = downloadIdentity(strings.ToLower(filepath.Clean(absolute)))
	part = filepath.Join(filepath.Dir(absolute), "."+filepath.Base(absolute)+"."+lockKey[:16]+".partial")
	return part, part + ".json", lockKey, nil
}

func validResumeState(state resumeState, source, integrity, validator string, size int64) bool {
	if state.Version != 1 || state.SourceHash != downloadIdentity(source) || state.Integrity != integrity || state.Validator != validator || state.Size != size || len(state.Completed) != parallelWorkers {
		return false
	}
	for i, count := range state.Completed {
		length := downloadSegmentBoundary(size, i+1) - downloadSegmentBoundary(size, i)
		if count < 0 || count > length {
			return false
		}
	}
	return true
}

func removeResumeFiles(part, statePath string) error {
	for _, p := range []string{part, statePath} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func discardResume(to string) error {
	part, statePath, key, err := resumePaths(to)
	if err != nil {
		return err
	}
	release, err := acquireDownloadLock(key)
	if err != nil {
		return err
	}
	defer release()
	for _, p := range []string{part, statePath} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func openResume(to, source, integrity, validator string, size int64) (*os.File, string, string, resumeState, func(), error) {
	var state resumeState
	part, statePath, key, err := resumePaths(to)
	if err != nil {
		return nil, "", "", state, nil, err
	}
	release, err := acquireDownloadLock(key)
	if err != nil {
		return nil, "", "", state, nil, err
	}
	fail := func(err error) (*os.File, string, string, resumeState, func(), error) {
		release()
		return nil, "", "", resumeState{}, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(part), 0o755); err != nil {
		return fail(err)
	}
	var data []byte
	var readErr error
	metaInfo, metaErr := os.Lstat(statePath)
	if metaErr != nil || !metaInfo.Mode().IsRegular() || metaInfo.Mode()&os.ModeSymlink != 0 || metaInfo.Size() > 64<<10 {
		readErr = fmt.Errorf("invalid download progress file")
	} else {
		data, readErr = os.ReadFile(statePath)
	}
	info, statErr := os.Lstat(part)
	valid := readErr == nil && json.Unmarshal(data, &state) == nil && validResumeState(state, source, integrity, validator, size) && statErr == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Size() == size
	if !valid {
		if err := removeResumeFiles(part, statePath); err != nil {
			return fail(err)
		}
		state = resumeState{Version: 1, SourceHash: downloadIdentity(source), Integrity: integrity, Validator: validator, Size: size, Completed: make([]int64, parallelWorkers)}
	}
	flags := os.O_RDWR
	if !valid {
		flags |= os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(part, flags, 0o600)
	if err != nil {
		return fail(err)
	}
	if valid {
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			file.Close()
			return fail(fmt.Errorf("download progress file changed"))
		}
	}
	if !valid {
		if err := file.Truncate(size); err != nil {
			file.Close()
			_ = removeResumeFiles(part, statePath)
			return fail(err)
		}
		if err := file.Sync(); err != nil {
			file.Close()
			_ = removeResumeFiles(part, statePath)
			return fail(err)
		}
		if err := writeResumeState(statePath, state); err != nil {
			file.Close()
			_ = removeResumeFiles(part, statePath)
			return fail(err)
		}
	}
	return file, part, statePath, state, release, nil
}

func writeResumeState(path string, state resumeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("save download progress: %w", err)
	}
	return nil
}
