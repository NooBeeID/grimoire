package openapi

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestT06_AC2_NoNetworkImports membuktikan package ini tidak bisa fetch atau membaca file.
func TestT06_AC2_NoNetworkImports(t *testing.T) {
	banned := map[string]bool{"net": true, "net/http": true, "net/url": true, "os": true, "io/fs": true,
		"io/ioutil": true, "path/filepath": true, "os/exec": true, "plugin": true}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v %d", err, len(files))
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if banned[p] || strings.HasPrefix(p, "net/") {
				t.Errorf("%s meng-import %q (dilarang)", f, p)
			}
		}
	}
}
