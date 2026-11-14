package swanpanel

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
)

func TestFrontendAssetsRequireBuiltJavaScriptAndHTML(t *testing.T) {
	const builtHTML = "<!doctype html><html><head><script type=\"module\" src=\"/assets/index-abcd.js\"></script></head></html>"
	cases := []struct {
		name  string
		files fs.FS
		valid bool
	}{
		{
			name: "valid Vite production bundle",
			files: fstest.MapFS{
				"index.html":           &fstest.MapFile{Data: []byte(builtHTML)},
				"assets/index-abcd.js": &fstest.MapFile{Data: []byte("console.log('ready')")},
			},
			valid: true,
		},
		{
			name:  "index missing",
			files: fstest.MapFS{"assets/index-abcd.js": &fstest.MapFile{Data: []byte("console.log(1)")}},
		},
		{
			name: "index empty",
			files: fstest.MapFS{
				"index.html":           &fstest.MapFile{Data: []byte(" ")},
				"assets/index-abcd.js": &fstest.MapFile{Data: []byte("console.log(1)")},
			},
		},
		{
			name: "development placeholder rejected",
			files: fstest.MapFS{
				"index.html":           &fstest.MapFile{Data: []byte("<!doctype html><html><!-- SWANPANEL_DEV_UI_PLACEHOLDER --></html>")},
				"assets/index-abcd.js": &fstest.MapFile{Data: []byte("console.log(1)")},
			},
		},
		{
			name:  "missing bundled JavaScript",
			files: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(builtHTML)}},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateFrontendAssets(test.files)
			if test.valid && err != nil {
				t.Fatalf("valid frontend bundle rejected: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("incomplete frontend bundle accepted")
			}
		})
	}
}

// A production bundle is built before go test in CI/release jobs.
// Local Go tests in the Vite Dev Container can use a dev placeholder instead.
func TestEmbeddedFrontendContainsProductionBuild(t *testing.T) {
	if os.Getenv("SWANPANEL_REQUIRE_BUILT_UI") != "1" {
		t.Skip("set SWANPANEL_REQUIRE_BUILT_UI=1 after building ui/ to check release assets")
	}
	if _, err := StaticFS(); err != nil {
		t.Fatalf("production frontend is missing from the Go binary: %v", err)
	}
}
