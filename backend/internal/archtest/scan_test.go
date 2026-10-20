package archtest

import (
	"os"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func normPkgPath(p string) (string, bool) {
	if i := strings.Index(p, " ["); i >= 0 {
		p = p[:i]
	}
	if strings.HasSuffix(p, ".test") {
		return "", false
	}
	p = strings.TrimSuffix(p, "_test")
	return p, true
}

// scanImports memuat seluruh paket di dir (termasuk _test.go dan tag integration).
func scanImports(t testing.TB, dir string) []Import {
	t.Helper()
	cfg := &packages.Config{
		Mode:       packages.NeedName | packages.NeedImports | packages.NeedFiles,
		Dir:        dir,
		Tests:      true,
		BuildFlags: []string{"-tags=integration"},
		Env:        append(os.Environ(), "GOWORK=off"),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	seen := map[Import]struct{}{}
	var out []Import
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			if e.Kind == packages.ListError || e.Kind == packages.ParseError {
				t.Fatalf("package error in %s: %v", pkg.ID, e)
			}
		}
		from, ok := normPkgPath(pkg.PkgPath)
		if !ok {
			continue
		}
		for to := range pkg.Imports {
			im := Import{From: from, To: to}
			if _, dup := seen[im]; dup {
				continue
			}
			seen[im] = struct{}{}
			out = append(out, im)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
