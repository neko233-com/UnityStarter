package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// logger writes what the app did, to a file in its data folder and, when the
// command line asks, to the terminal.
var logger = log.New(io.Discard, "", log.LstdFlags)

// startLogging opens the app's log, rotating it when it passes a megabyte.
func startLogging(verbose bool) error {
	dir := filepath.Join(userDataDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "app.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
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
