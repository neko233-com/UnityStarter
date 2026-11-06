package main

import "strings"

type moduleLayoutRow struct {
	Module editorModule
	Depth  int
}

type moduleLayoutGroup struct {
	Kind string
	Rows []moduleLayoutRow
}

func moduleGroupKind(m editorModule) string {
	category := strings.ToLower(m.Category)
	switch {
	case strings.Contains(category, "dev") || strings.HasPrefix(m.ID, "visualstudio"):
		return "tools"
	case strings.Contains(category, "lang") || strings.HasPrefix(m.ID, "language-"):
		return "languages"
	case strings.Contains(category, "doc") || m.ID == "documentation":
		return "docs"
	case strings.Contains(category, "platform") || moduleOutput(m) != "":
		return "platforms"
	default:
		return "other"
	}
}

// Children stay directly below their visible parent even when the catalog
// stores them elsewhere. A visited set also handles malformed cycles.
func moduleLayout(modules []editorModule) []moduleLayoutGroup {
	visible := map[string]bool{}
	for _, m := range modules {
		if m.Visible {
			visible[m.ID] = true
		}
	}
	groups := []moduleLayoutGroup{{Kind: "tools"}, {Kind: "platforms"}, {Kind: "docs"}, {Kind: "languages"}, {Kind: "other"}}
	visited := map[string]bool{}
	var appendTree func(*moduleLayoutGroup, editorModule, int)
	appendTree = func(group *moduleLayoutGroup, m editorModule, depth int) {
		if visited[m.ID] {
			return
		}
		visited[m.ID] = true
		group.Rows = append(group.Rows, moduleLayoutRow{Module: m, Depth: depth})
		for _, child := range modules {
			if child.Visible && child.Parent == m.ID {
				appendTree(group, child, depth+1)
			}
		}
	}
	for i := range groups {
		for _, m := range modules {
			if m.Visible && (!visible[m.Parent] || m.Parent == "") && moduleGroupKind(m) == groups[i].Kind {
				appendTree(&groups[i], m, 0)
			}
		}
	}
	for _, m := range modules {
		if m.Visible && !visited[m.ID] {
			appendTree(&groups[len(groups)-1], m, 0)
		}
	}
	return groups
}

func selectedModuleDownloadSize(modules []editorModule, selected map[string]bool) (int64, bool) {
	var total int64
	known := true
	for _, m := range modules {
		if !selected[m.ID] || m.Selected {
			continue
		}
		if m.DownloadSize <= 0 || total > (1<<63-1)-m.DownloadSize {
			known = false
			continue
		}
		total += m.DownloadSize
	}
	return total, known
}
