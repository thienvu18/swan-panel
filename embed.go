package swanpanel

import (
	"embed"
	"io/fs"
)

//go:embed all:ui/dist
var distFS embed.FS

// StaticFS returns the embedded filesystem rooted at the built UI dist directory
func StaticFS() (fs.FS, error) {
	return fs.Sub(distFS, "ui/dist")
}
