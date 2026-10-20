package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func repoRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller gagal")
	}
	// backend/internal/archtest/repo_test.go -> backend
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func TestT00_AC4_RepoIsClean(t *testing.T) {
	edges := scanImports(t, repoRoot(t))
	froms := map[string]bool{}
	for _, e := range edges {
		froms[e.From] = true
	}
	for _, want := range []string{"grimoire/internal/archtest", "grimoire/internal/domain/core"} {
		if !froms[want] {
			t.Fatalf("pemindaian kosong/tidak lengkap: tidak ada edge dari %s", want)
		}
	}
	for _, v := range Check(edges) {
		t.Errorf("%s", v)
	}
}

func TestModulePathMatchesGoMod(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`).FindSubmatch(b)
	if m == nil || string(m[1]) != ModulePath {
		t.Fatalf("go.mod module = %q, want %q", m, ModulePath)
	}
}

func TestRuleCatalog_ExistsInRulesTxt(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "..", "rules.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{Rule41, Rule42, Rule43, Rule45, Rule46, Rule49} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(id) + `\s`).Match(b) {
			t.Errorf("aturan %s tidak ada di rules.txt", id)
		}
	}
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestT00_AC4_ScanFindsViolationsInTempModule(t *testing.T) {
	if testing.Short() {
		t.Skip("memuat modul sementara")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module grimoire\n\ngo 1.23\n")
	pk := func(rel string, imports ...string) string {
		var sb strings.Builder
		sb.WriteString("package " + filepath.Base(rel) + "\n")
		for _, i := range imports {
			sb.WriteString("import _ \"" + i + "\"\n")
		}
		return sb.String()
	}
	stub := func(rel string) { write(t, root, rel+"/stub.go", pk(rel)) }
	for _, d := range []string{
		"internal/domain/core",
		"internal/domain/project/internal/knowledge/base",
		"internal/domain/project/internal/knowledge/search",
		"internal/domain/core/internal/auth/logout",
		"internal/domain/project",
		"internal/infra/bus",
	} {
		stub(d)
	}
	// "internal/domain/project" harus berupa paket; "core" diimpor oleh a.go.
	write(t, root, "internal/domain/project/a.go", pk("internal/domain/project", "grimoire/internal/domain/core"))
	write(t, root, "internal/domain/project/internal/apikey/x.go", pk("internal/domain/project/internal/apikey", "grimoire/internal/domain/project/internal/knowledge/base"))
	write(t, root, "internal/domain/project/internal/knowledge/upload/u.go", pk("internal/domain/project/internal/knowledge/upload", "grimoire/internal/domain/project/internal/knowledge/search"))
	write(t, root, "internal/domain/core/internal/auth/login/l.go", "package login\n")
	write(t, root, "internal/domain/core/internal/auth/login/l_test.go", pk("internal/domain/core/internal/auth/login", "grimoire/internal/domain/core/internal/auth/logout"))
	write(t, root, "internal/infra/bus/b.go", pk("internal/infra/bus", "grimoire/internal/domain/core"))
	write(t, root, "lib/contracts/core/c.go", pk("lib/contracts/core", "grimoire/internal/infra/bus"))
	write(t, root, "lib/util/u.go", pk("lib/util", "grimoire/internal/infra/bus"))
	write(t, root, "internal/domain/core/internal/auth/withtag/w.go", "//go:build integration\n\n"+pk("internal/domain/core/internal/auth/withtag", "grimoire/internal/domain/project"))

	got := map[string]bool{}
	for _, v := range Check(scanImports(t, root)) {
		got[v.Rule+"|"+v.From+"|"+v.To] = true
	}
	want := []string{
		"4.1|grimoire/internal/domain/project|grimoire/internal/domain/core",
		"4.2|grimoire/internal/domain/project/internal/apikey|grimoire/internal/domain/project/internal/knowledge/base",
		"4.3|grimoire/internal/domain/project/internal/knowledge/upload|grimoire/internal/domain/project/internal/knowledge/search",
		"4.3|grimoire/internal/domain/core/internal/auth/login|grimoire/internal/domain/core/internal/auth/logout",
		"4.6|grimoire/internal/infra/bus|grimoire/internal/domain/core",
		"4.5|grimoire/lib/contracts/core|grimoire/internal/infra/bus",
		"4.9|grimoire/lib/util|grimoire/internal/infra/bus",
		"4.1|grimoire/internal/domain/core/internal/auth/withtag|grimoire/internal/domain/project",
	}
	var missing, extra []string
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
		if !got[w] {
			missing = append(missing, w)
		}
	}
	for g := range got {
		if !wantSet[g] {
			extra = append(extra, g)
		}
	}
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("missing=%v extra=%v", missing, extra)
	}
}
