package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// vcsInfo is the version control a project is kept in.
type vcsInfo struct {
	Kind   string // "git", "svn" or "lore"
	Branch string
	Remote string
	Dirty  bool
}

// detectVCS reads what the project's folder says about its version control,
// without walking the history.
func detectVCS(dir string) vcsInfo {
	if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		v := vcsInfo{Kind: "git"}
		gitDir := filepath.Join(dir, ".git")
		if !info.IsDir() {
			// A worktree's .git is a file naming its folder in the repo.
			if data, err := os.ReadFile(gitDir); err == nil {
				if rest, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
					gitDir = strings.TrimSpace(rest)
					if !filepath.IsAbs(gitDir) {
						gitDir = filepath.Join(dir, gitDir)
					}
				}
			}
		}
		v.Branch = gitBranch(gitDir)
		v.Remote = gitRemote(gitDir)
		v.Dirty = gitDirty(dir)
		return v
	}
	if _, err := os.Stat(filepath.Join(dir, ".svn")); err == nil {
		return vcsInfo{Kind: "svn", Remote: svnURL(dir)}
	}
	if info, err := os.Stat(filepath.Join(dir, ".lore")); err == nil && info.IsDir() {
		return vcsInfo{Kind: "lore"}
	}
	return vcsInfo{}
}

func gitBranch(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(data))
	if rest, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok {
		return rest
	}
	if len(ref) > 8 {
		return ref[:8]
	}
	return ref
}

func gitRemote(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	section := ""
	first := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		url, ok := strings.CutPrefix(line, "url =")
		if !ok {
			continue
		}
		url = strings.TrimSpace(url)
		if first == "" {
			first = url
		}
		if section == `[remote "origin"]` {
			return url
		}
	}
	return first
}

// gitDirty reports whether the project has changes to tracked files.
func gitDirty(dir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "--untracked-files=no")
	hideWindowCmd(cmd)
	out, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func svnURL(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "svn", "info", "--show-item", "url", dir)
	hideWindowCmd(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// vcsCache keeps what a scan read, so that a refresh does not run git for
// every project again.
var vcsCache = struct {
	sync.Mutex
	entries map[string]vcsCacheEntry
}{entries: map[string]vcsCacheEntry{}}

type vcsCacheEntry struct {
	info vcsInfo
	at   time.Time
}

func detectVSCCached(dir string) vcsInfo {
	vcsCache.Lock()
	entry, ok := vcsCache.entries[dir]
	vcsCache.Unlock()
	if ok && time.Since(entry.at) < 20*time.Second {
		return entry.info
	}
	info := detectVCS(dir)
	vcsCache.Lock()
	vcsCache.entries[dir] = vcsCacheEntry{info: info, at: time.Now()}
	vcsCache.Unlock()
	return info
}

// detectPipeline names the render pipeline of a project, from the packages it
// uses.
func detectPipeline(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "Packages", "manifest.json"))
	if err != nil {
		return ""
	}
	manifest := string(data)
	switch {
	case strings.Contains(manifest, `"com.unity.render-pipelines.high-definition"`):
		return "hdrp"
	case strings.Contains(manifest, `"com.unity.render-pipelines.universal"`):
		return "urp"
	}
	return "builtin"
}
