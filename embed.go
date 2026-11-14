package swanpanel

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:ui/dist
var distFS embed.FS

// StaticFS returns the compiled React application, not the development
// placeholder that exists solely to satisfy go:embed when Vite serves the UI.
func StaticFS() (fs.FS, error) {
	frontend, err := fs.Sub(distFS, "ui/dist")
	if err != nil {
		return nil, fmt.Errorf("embedded frontend: %w", err)
	}
	if err := validateFrontendAssets(frontend); err != nil {
		return nil, err
	}
	return frontend, nil
}

func validateFrontendAssets(frontend fs.FS) error {
	html, err := fs.ReadFile(frontend, "index.html")
	if err != nil {
		return fmt.Errorf("embedded frontend index.html unavailable: %w", err)
	}
	if len(bytes.TrimSpace(html)) == 0 || !bytes.Contains(bytes.ToLower(html), []byte("<html")) {
		return fmt.Errorf("embedded frontend index.html is empty or invalid")
	}
	if bytes.Contains(html, []byte("SWANPANEL_DEV_UI_PLACEHOLDER")) {
		return fmt.Errorf("embedded frontend is a development placeholder, not a production build")
	}
	jsFiles, err := fs.Glob(frontend, "assets/*.js")
	if err != nil {
		return fmt.Errorf("list embedded frontend JavaScript bundles: %w", err)
	}
	if len(jsFiles) == 0 {
		return fmt.Errorf("embedded frontend contains no bundled JavaScript; run npm run build in ui/")
	}
	return nil
}
