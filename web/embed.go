// Package web embeds the built Web UI. The React app lives in this directory;
// npm run build writes dist/, and go build embeds whatever is there. A binary
// built before the UI serves a note in its place.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built UI, or nil when dist holds no index.html.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
