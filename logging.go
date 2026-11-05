package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// logger writes what the app did, to a file in its data folder and, when the
// command line asks, to the terminal.
var logger = log.New(io.Discard, "", log.LstdFlags)

// Logs are rotated at a megabyte, and only the newest few kept.
const (
	logMaxBytes = 1 << 20
	logKeep     = 5
	logMaxAge   = 14 * 24 * time.Hour
)

// startLogging opens the app's log, rotating it when it passes a megabyte and
// pruning the logs that are old.
func startLogging(verbose bool) error {
	dir := filepath.Join(userDataDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "app.log")
	if info, err := os.Stat(path); err == nil && info.Size() > logMaxBytes {
		rotated := fmt.Sprintf("%s.%s", path, time.Now().Format("20060102-150405"))
		_ = os.Rename(path, rotated)
	}
	pruneLogs(dir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	var out io.Writer = file
	if verbose {
		out = io.MultiWriter(file, os.Stderr)
	}
	logger = log.New(out, "", log.LstdFlags)
	return nil
}

// pruneLogs keeps the newest rotated logs, and forgets the old ones.
func pruneLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var rotated []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "app.log.") {
			rotated = append(rotated, entry)
		}
	}
	sort.Slice(rotated, func(i, j int) bool {
		a, errA := rotated[i].Info()
		b, errB := rotated[j].Info()
		if errA != nil || errB != nil {
			return rotated[i].Name() > rotated[j].Name()
		}
		return a.ModTime().After(b.ModTime())
	})
	for i, entry := range rotated {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if i >= logKeep || time.Since(info.ModTime()) > logMaxAge {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func logPath() string {
	return filepath.Join(userDataDir(), "logs")
}

// startConsoleLogging writes what a command does to the terminal, when it is
// asked to.
func startConsoleLogging(verbose bool) {
	if verbose {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
}
