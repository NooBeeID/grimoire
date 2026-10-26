//go:build integration

package store

import (
	"fmt"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
)

func put(t *testing.T, s *Store, pid, path, content string) {
	t.Helper()
	f := base.NewFile(pid, path, content, base.Actor{ID: user, Label: "Budi"})
	if err := s.InsertFiles(ctx, []base.File{f}); err != nil {
		t.Fatalf("put %s: %v", path, err)
	}
}

func runSearch(t *testing.T, s *Store, pid, q string, syn [][]string) ([]base.SearchHit, int) {
	t.Helper()
	terms, ok := base.ParseTerms(q)
	if !ok {
		return nil, 0
	}
	plan := base.NewSearchPlan(terms, syn)
	if c := plan.TypoCandidates(); len(c) > 0 {
		sugg, err := s.SuggestTerms(ctx, pid, c)
		if err != nil {
			t.Fatalf("suggest: %v", err)
		}
		plan = plan.WithTypos(sugg)
	}
	hits, total, err := s.Search(ctx, pid, plan, "%", 20)
	if err != nil {
		t.Fatalf("search %q: %v", q, err)
	}
	return hits, total
}

func paths(hits []base.SearchHit) string {
	var p []string
	for _, h := range hits {
		p = append(p, h.Path)
	}
	return strings.Join(p, ",")
}

// T20: variasi kata lewat stemming Indonesia/Inggris dan peluluhan K/P/T/S.
func TestSearch_Stem(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "ops/paket.md", "# Layanan\nPengiriman paket dikirim setiap hari.")
	put(t, s, p, "ops/rilis.md", "# Rilis\nSetiap deployment dijalankan lewat pipeline.")
	put(t, s, p, "ops/lain.md", "# Lain\nTidak relevan sama sekali.")

	for _, q := range []string{"mengirim", "kirim", "dikirim"} {
		if h, _ := runSearch(t, s, p, q, nil); paths(h) != "ops/paket.md" {
			t.Errorf("%q -> %s", q, paths(h))
		}
	}
	for _, q := range []string{"deploy", "deploys", "deployed"} {
		if h, _ := runSearch(t, s, p, q, nil); paths(h) != "ops/rilis.md" {
			t.Errorf("%q -> %s", q, paths(h))
		}
	}
}

func TestSearch_TitleOutranksBody(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "a-isi.md", "# Catatan\nmembahas webhook di tengah paragraf panjang.")
	put(t, s, p, "b-judul.md", "# Webhook\nisi lain.")
	put(t, s, p, "docs/webhook.md", "tanpa judul")
	h, total := runSearch(t, s, p, "webhook", nil)
	if total != 3 || len(h) != 3 || h[2].Path != "a-isi.md" {
		t.Fatalf("hits = %s total=%d", paths(h), total)
	}
}

func TestSearch_HeadingWeightAndSection(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "ops/runbook.md", "# Runbook\nPembuka umum.\n\n## Rollback cepat\nJalankan perintah revert lalu rollback.\n\n## Penutup\nSelesai.")
	put(t, s, p, "ops/lain.md", "# Lain\nada kata rollback di isi saja.")
	h, _ := runSearch(t, s, p, "rollback", nil)
	if len(h) != 2 || h[0].Path != "ops/runbook.md" {
		t.Fatalf("hits = %s", paths(h))
	}
	if h[0].Section != "Rollback cepat" {
		t.Fatalf("section = %q", h[0].Section)
	}
	if !strings.Contains(h[0].Headline, base.HighlightStart) {
		t.Fatalf("headline tanpa sorotan: %q", h[0].Headline)
	}
	if h[1].Section != "Lain" {
		t.Fatalf("section dokumen kedua = %q", h[1].Section)
	}
}

func TestSearch_Coverage(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "both.md", "# Doc\nreset password pengguna")
	put(t, s, p, "one.md", "# Doc\nhanya password saja")
	h, _ := runSearch(t, s, p, "reset password", nil)
	if paths(h) != "both.md,one.md" {
		t.Fatalf("2 kata = %s", paths(h))
	}
	put(t, s, p, "single.md", "# Doc\nhanya satu: reset")
	h, _ = runSearch(t, s, p, "reset password pengguna", nil)
	if paths(h) != "both.md,one.md" && paths(h) != "both.md" {
		t.Fatalf("3 kata = %s", paths(h))
	}
	for _, x := range h {
		if x.Path == "single.md" {
			t.Fatalf("dokumen dengan 1 dari 3 kata tidak boleh muncul: %s", paths(h))
		}
	}
}

func TestSearch_PhraseBoost(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "a.md", "# Catatan\nexponential naik. lalu ada backoff di tempat lain.")
	put(t, s, p, "b.md", "# Retry\n## Exponential backoff\nstrategi tunggu.")
	h, _ := runSearch(t, s, p, `"exponential backoff"`, nil)
	if len(h) == 0 || h[0].Path != "b.md" {
		t.Fatalf("hits = %s", paths(h))
	}
}

func TestSearch_SynonymAndExpandedPlan(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "auth.md", "# Autentikasi\nProses autentikasi pengguna.")
	if h, _ := runSearch(t, s, p, "login", nil); len(h) != 0 {
		t.Fatalf("tanpa kamus: %s", paths(h))
	}
	h, _ := runSearch(t, s, p, "login", [][]string{{"login", "autentikasi"}})
	if paths(h) != "auth.md" {
		t.Fatalf("dengan kamus: %s", paths(h))
	}
}

func TestSuggestTerms_Typo(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	put(t, s, a, "ops/rollback.md", "# Rollback\nlangkah.")
	put(t, s, b, "ops/lain.md", "# Pagination\nproject lain.")

	got, err := s.SuggestTerms(ctx, a, []string{"rollbak", "rollback", "pagnation"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got["rollbak"]) != "[rollback]" {
		t.Fatalf("rollbak = %v", got["rollbak"])
	}
	if _, ok := got["rollback"]; ok {
		t.Fatalf("kata yang sudah cocok tidak boleh disarankan: %v", got)
	}
	if _, ok := got["pagnation"]; ok {
		t.Fatalf("project lain bocor: %v", got)
	}
	if m, err := s.SuggestTerms(ctx, "bukan-uuid", []string{"x"}); err != nil || len(m) != 0 {
		t.Fatalf("%v %v", m, err)
	}
	if m, err := s.SuggestTerms(ctx, a, nil); err != nil || len(m) != 0 {
		t.Fatalf("%v %v", m, err)
	}

	h, _ := runSearch(t, s, a, "rollbak", nil)
	if paths(h) != "ops/rollback.md" {
		t.Fatalf("typo end-to-end = %s", paths(h))
	}
}

// Review Focus 3: baris lama (sections NULL) tetap tercari; section kosong.
func TestSearch_NullSections(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "old.md", "# Lama\nkonfigurasi tersimpan di sini.")
	if _, err := d.ExecContext(ctx, `UPDATE knowledge_files SET sections = NULL WHERE project_id = $1`, p); err != nil {
		t.Fatal(err)
	}
	h, _ := runSearch(t, s, p, "konfigurasi", nil)
	if len(h) != 1 || h[0].Section != "" || !strings.Contains(h[0].Headline, base.HighlightStart) {
		t.Fatalf("hits = %+v", h)
	}
}

// Review Focus 2: offset rune; cuplikan dipotong tepat di heading meski ada emoji sebelumnya.
func TestSearch_SectionSnippetMultibyte(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "emoji.md", "😀😀😀 pembuka\n# Awal\nteks awal\n## Kebijakan retensi\nlog disimpan 30 hari untuk retensi.\n## Akhir\npenutup")
	h, _ := runSearch(t, s, p, "retensi", nil)
	if len(h) != 1 || h[0].Section != "Kebijakan retensi" {
		t.Fatalf("hits = %+v", h)
	}
	if strings.Contains(h[0].Headline, "😀") {
		t.Fatalf("cuplikan harus dari bagian heading, bukan awal dokumen: %q", h[0].Headline)
	}
}

// Review Focus 1: hanya operator / stopword tidak membuat error SQL.
func TestSearch_OperatorOnlyAndStopword(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "a.md", "# A\nthe quick brown fox")
	for _, q := range []string{"-", "!", ":*", `""`, "&|", "'"} {
		if h, total := runSearch(t, s, p, q, nil); len(h) != 0 || total != 0 {
			t.Errorf("%q -> %v %d", q, paths(h), total)
		}
	}
	if _, _, err := s.Search(ctx, p, base.SearchPlan{Groups: []base.QueryGroup{{Alts: []base.Alt{{Text: "the"}}}}}, "%", 5); err != nil {
		t.Fatalf("stopword: %v", err)
	}
	if _, _, err := s.Search(ctx, p, base.SearchPlan{Groups: []base.QueryGroup{{Alts: []base.Alt{{Text: ""}}}}}, "%", 5); err != nil {
		t.Fatalf("alternatif kosong: %v", err)
	}
	if h, total := runSearch(t, s, p, "the", nil); total > 1 || len(h) > 1 {
		t.Fatalf("the -> %v", paths(h))
	}
}

func TestSearch_ProjectIsolation(t *testing.T) {
	d, s := setup(t)
	a, b := newProject(t, d, "Alpha"), newProject(t, d, "Bravo")
	put(t, s, a, "docs/auth/login.md", "# Login\nrahasia")
	put(t, s, a, "other/x.md", "# X\nrahasia")
	put(t, s, b, "docs/uniqueb.md", "# B\nuniqueb")
	if h, _ := runSearch(t, s, a, "uniqueb", nil); len(h) != 0 {
		t.Fatalf("bocor: %s", paths(h))
	}
	terms, _ := base.ParseTerms("rahasia")
	h, total, err := s.Search(ctx, a, base.NewSearchPlan(terms, nil), base.LikePrefix("docs/auth"), 10)
	if err != nil || total != 1 || paths(h) != "docs/auth/login.md" {
		t.Fatalf("prefix: %s %d %v", paths(h), total, err)
	}
	if h, total, err := s.Search(ctx, "bukan-uuid", base.NewSearchPlan(terms, nil), "%", 10); err != nil || total != 0 || len(h) != 0 {
		t.Fatalf("non-uuid: %v %d %v", h, total, err)
	}
	if h, total, err := s.Search(ctx, a, base.SearchPlan{}, "%", 10); err != nil || total != 0 || len(h) != 0 {
		t.Fatalf("plan kosong: %v %d %v", h, total, err)
	}
}

func TestSearch_LimitAndTotal(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	for i := 0; i < 5; i++ {
		put(t, s, p, fmt.Sprintf("f%d.md", i), "# F\nkeyword umum")
	}
	terms, _ := base.ParseTerms("keyword")
	h, total, err := s.Search(ctx, p, base.NewSearchPlan(terms, nil), "%", 2)
	if err != nil || len(h) != 2 || total != 5 {
		t.Fatalf("%d %d %v", len(h), total, err)
	}
}

func TestInsertUpdateWritesSections(t *testing.T) {
	d, s := setup(t)
	p := newProject(t, d, "Alpha")
	put(t, s, p, "a.md", "# Satu\nx\n## Dua\ny")
	var n int
	if err := d.GetContext(ctx, &n, `SELECT jsonb_array_length(sections) FROM knowledge_files WHERE project_id = $1`, p); err != nil || n != 2 {
		t.Fatalf("insert sections = %d %v", n, err)
	}
	f, _, _ := s.FindByPath(ctx, p, "a.md")
	f = f.WithContent("# Hanya\nz", base.Actor{ID: user, Label: "Budi"})
	if err := s.UpdateFiles(ctx, []base.File{f}); err != nil {
		t.Fatal(err)
	}
	var h string
	if err := d.GetContext(ctx, &h, `SELECT sections->0->>'h' FROM knowledge_files WHERE project_id = $1`, p); err != nil || h != "Hanya" {
		t.Fatalf("update sections = %q %v", h, err)
	}
	// File tanpa SectionsJSON (dibangun manual) tetap menulis sections dari isi.
	g := base.File{ProjectID: p, Path: "b.md", Title: "B", Content: "# B\nx", Hash: base.Hash("# B\nx"), SizeBytes: 5, UpdatedBy: user, UpdatedByLabel: "Budi"}
	if err := s.InsertFiles(ctx, []base.File{g}); err != nil {
		t.Fatal(err)
	}
	if err := d.GetContext(ctx, &n, `SELECT jsonb_array_length(sections) FROM knowledge_files WHERE project_id = $1 AND path='b.md'`, p); err != nil || n != 1 {
		t.Fatalf("fallback sections = %d %v", n, err)
	}
}
