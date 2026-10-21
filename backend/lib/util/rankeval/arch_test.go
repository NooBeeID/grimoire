package rankeval

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func forbiddenImports(src string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, im := range f.Imports {
		p := strings.Trim(im.Path.Value, `"`)
		if strings.HasPrefix(p, "grimoire/internal") || strings.HasPrefix(p, "grimoire/cmd") {
			bad = append(bad, p)
		}
	}
	return bad, nil
}

func TestForbiddenImportsDetector(t *testing.T) {
	bad, err := forbiddenImports("package x\nimport (\n\t\"fmt\"\n\t\"grimoire/internal/infra/bus\"\n\t\"grimoire/cmd/foo\"\n)\n")
	if err != nil || len(bad) != 2 {
		t.Fatalf("bad=%v err=%v", bad, err)
	}
	bad, _ = forbiddenImports("package x\nimport \"grimoire/lib/constants\"\n")
	if len(bad) != 0 {
		t.Errorf("lib boleh: %v", bad)
	}
}

func TestNoInternalImports(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		bad, err := forbiddenImports(string(b))
		if err != nil {
			t.Fatal(err)
		}
		if len(bad) > 0 {
			t.Errorf("%s mengimpor %v", f, bad)
		}
	}
}
