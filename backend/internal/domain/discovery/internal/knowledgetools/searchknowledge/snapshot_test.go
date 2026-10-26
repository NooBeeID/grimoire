package searchknowledge

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/project/knowledge/search"
)

var updateGolden = flag.Bool("update-golden", false, "tulis ulang testdata/*.golden")

type snapCase struct {
	name, query, prefix string
	resp                search.Response
}

var snapCases = []snapCase{
	{
		name: "three_hits", query: "rate limit", prefix: "docs",
		resp: search.Response{Total: 3, Items: []search.Item{
			{Path: "docs/guides/b.md", Score: 0.6079, Snippet: "**rate**\n**limiting** per key"},
			{Path: "docs/guides/c.md", Score: 0.4, Snippet: ""},
			{Path: "docs/d.md", Score: 0.12345, Snippet: "batas **rate** harian"},
		}},
	},
	{
		name: "more_hits_hint", query: "a", prefix: "",
		resp: search.Response{Total: 9, Items: []search.Item{
			{Path: "a.md", Score: 0.9, Snippet: "**a** satu"},
			{Path: "b/a.md", Score: 0.5, Snippet: "**a** dua"},
		}},
	},
	{
		name: "section_expanded", query: "login", prefix: "",
		resp: search.Response{Total: 2, Expanded: true, Items: []search.Item{
			{Path: "auth/autentikasi.md", Score: 0.8, Snippet: "alur **autentikasi**", Section: "Alur login"},
			{Path: "auth/lain.md", Score: 0.3, Snippet: "tanpa bagian"},
		}},
	},
	{name: "no_hits", query: "x", prefix: "docs/guides", resp: search.Response{}},
}

func goldenPath(name, kind string) string {
	return filepath.Join("testdata", name+"."+kind+".golden")
}

func structuredGolden(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

func compareGolden(t *testing.T, name, kind, got string) {
	t.Helper()
	p := goldenPath(name, kind)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("%s berubah.\nwant:\n%s\ngot:\n%s", p, want, got)
	}
}

// Golden dibuat dari commit sebelum T11, sehingga tes ini membuktikan teks dan
// structuredContent untuk agent tidak berubah (AC5).
func TestSnapshot_TextAndStructuredUnchanged(t *testing.T) {
	for _, c := range snapCases {
		t.Run(c.name, func(t *testing.T) {
			h := base.NewSearchHits(c.query, c.prefix, c.resp)
			compareGolden(t, c.name, "text", h.Text())
			compareGolden(t, c.name, "structured", structuredGolden(t, h.Structured()))
			if *updateGolden {
				return
			}

			// Jalur handler tidak boleh menambah apa pun pada Text/Structured.
			f := newFakeBus(true)
			f.resp = c.resp
			args := `{"query":` + jsonStr(c.query)
			if c.prefix != "" {
				args += `,"path":` + jsonStr(c.prefix)
			}
			args += `}`
			r, err := f.call("p1", args)
			if err != nil || r.IsError {
				t.Fatalf("%+v %v", r, err)
			}
			wantText, _ := os.ReadFile(goldenPath(c.name, "text"))
			if r.Text != string(wantText) {
				t.Fatalf("text handler:\n%s\nwant:\n%s", r.Text, wantText)
			}
			if got := structuredGolden(t, r.Structured); got != structuredFromFile(t, c.name) {
				t.Fatalf("structured handler:\n%s", got)
			}
			var a, b map[string]any
			_ = json.Unmarshal([]byte(structuredFromFile(t, c.name)), &a)
			bb, _ := json.Marshal(r.Structured)
			_ = json.Unmarshal(bb, &b)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("structured: %v != %v", a, b)
			}
		})
	}
}

func structuredFromFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(goldenPath(name, "structured"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
