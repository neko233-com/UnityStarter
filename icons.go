package main

import "github.com/egoist/mygo/ui"

// The sidebar's icons, drawn in the color of the text.
var (
	iconProjects = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M4 5h6l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zm0 5v8h16v-8H4z"/></svg>`))
	iconInstalls = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M3 13h5l1 2h6l1-2h5v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6zm9-11 4.5 4.5-1.4 1.4L13 5.8V12h-2V5.8L8.9 7.9 7.5 6.5 12 2z"/></svg>`))
	iconSettings = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M3 6h10a3 3 0 0 1 5.8 0H22v2h-3.2a3 3 0 0 1-5.8 0H3V6zm0 10h3.2a3 3 0 0 1 5.8 0H22v2h-10a3 3 0 0 1-5.8 0H3v-2z"/></svg>`))
)
