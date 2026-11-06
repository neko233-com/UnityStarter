package main

import "regexp"

// ProductVersion is typically 7000.0.0a16_0745a91df592. Keep the revision
// separate from the editor version that projects and release catalogs use.
var editorProductVersionPattern = regexp.MustCompile(`(?i)^(\d+\.\d+\.\d+[abfpt]\d+)(?:_[0-9a-f]+)?$`)

func parseEditorProductVersion(product string) string {
	match := editorProductVersionPattern.FindStringSubmatch(product)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}
