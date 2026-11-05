package main

import "github.com/egoist/mygo/ui"

// The sidebar's icons, drawn in the color of the text.
var (
	iconProjects = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M4 5h6l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2zm0 5v8h16v-8H4z"/></svg>`))
	iconInstalls = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M3 13h5l1 2h6l1-2h5v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6zm9-11 4.5 4.5-1.4 1.4L13 5.8V12h-2V5.8L8.9 7.9 7.5 6.5 12 2z"/></svg>`))
	iconSettings = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M3 6h10a3 3 0 0 1 5.8 0H22v2h-3.2a3 3 0 0 1-5.8 0H3V6zm0 10h3.2a3 3 0 0 1 5.8 0H22v2h-10a3 3 0 0 1-5.8 0H3v-2z"/></svg>`))
	iconStore    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M5 4h14l2 4H3l2-4zm-1 6h16v9a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-9zm5 2v1a3 3 0 0 0 6 0v-1h-2v1a1 1 0 0 1-2 0v-1H9z"/></svg>`))
	iconPublish  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M12 2c2.8 2.4 4 5.6 4 9.2l2.4 3.2-3.2.8-1.2 2.4h-4l-1.2-2.4-3.2-.8L8 11.2C8 7.6 9.2 4.4 12 2zm0 5.5a1.7 1.7 0 1 0 0 3.4 1.7 1.7 0 0 0 0-3.4zM7 19h10v2H7v-2z"/></svg>`))
	iconLicense  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M20 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2zm0 4H4V6h16v2zm-7 3h5v2h-5v-2zm0 4h5v2h-5v-2zM5 13h6v4H5v-4z"/></svg>`))
	iconDocs     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M11 5.5V20c-1.6-1.3-3.3-2-5.5-2H4V4h1.5c2.2 0 3.9.5 5.5 1.5zm2 0C14.6 4.5 16.3 4 18.5 4H20v14h-1.5c-2.2 0-3.9.7-5.5 2V5.5z"/></svg>`))
	iconFeedback = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M4 4h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-5 4V6a2 2 0 0 1 2-2zm3 5v2h10V9H7zm0 4v2h7v-2H7z"/></svg>`))
	iconDownload = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M11 3h2v8.2l2.9-2.9 1.4 1.4L12 15l-5.3-5.3 1.4-1.4L11 11.2V3zM4 17h16v2H4v-2z"/></svg>`))
)
