package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type recentEditorProject struct {
	Path string
	Kind string
}

// Unity hashes EditorPrefs value names with this 32-bit djb2-xor variant.
func editorPrefValueName(index int) string {
	name := fmt.Sprintf("RecentlyUsedProjectPaths-%d", index)
	hash := uint32(5381)
	for _, b := range []byte(name) {
		hash = hash*33 ^ uint32(b)
	}
	return fmt.Sprintf("%s_h%d", name, hash)
}

func collectRecentEditorProjects(kind string, lookup func(string) []byte) []recentEditorProject {
	var out []recentEditorProject
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		data := lookup(editorPrefValueName(i))
		if !utf8.Valid(data) {
			continue
		}
		path := strings.TrimSpace(strings.TrimRight(string(data), "\x00"))
		if path == "" || strings.ContainsRune(path, '\x00') {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, recentEditorProject{Path: path, Kind: kind})
	}
	return out
}
