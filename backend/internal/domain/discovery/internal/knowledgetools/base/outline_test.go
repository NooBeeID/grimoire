package base

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/project/knowledge/readfile"
)

var ts = time.Date(2026, 10, 24, 9, 30, 0, 0, time.UTC)

func resp(content string) readfile.Response {
	return readfile.Response{Found: true, Content: content, SizeBytes: len(content), TotalBytes: len(content), UpdatedAt: ts}
}

func TestIsMarkdown(t *testing.T) {
	for p, want := range map[string]bool{"a.md": true, "A.MD": true, "x.markdown": true, "a.txt": false, "md": false} {
		if IsMarkdown(p) != want {
			t.Errorf("%s -> %v", p, !want)
		}
	}
}

func TestNewOutline_TextAndStructured(t *testing.T) {
	o := NewOutline("docs/a.md", resp("# A\nx\n## B\n"), 4096)
	txt := o.Text()
	for _, w := range []string{"docs/a.md (", "diperbarui 2026-10-24 09:30 UTC", "2 heading", "L1 # A [0-", "  L3 ## B [", "Baca satu bagian dengan read_section(path, heading)"} {
		if !strings.Contains(txt, w) {
			t.Errorf("text tanpa %q:\n%s", w, txt)
		}
	}
	st := o.Structured()
	hs, ok := st["headings"].([]map[string]any)
	if !ok || len(hs) != 2 {
		t.Fatalf("headings = %#v", st["headings"])
	}
	if st["updated_at"] != "2026-10-24T09:30:00Z" || st["total_headings"] != 2 || st["truncated"] != false {
		t.Fatalf("st = %v", st)
	}
	if _, err := json.Marshal(st); err != nil {
		t.Fatal(err)
	}
	r := o.Result()
	if r.IsError || r.TargetType != "file" || r.Target != "docs/a.md" {
		t.Fatalf("result = %+v", r)
	}
}

func TestNewOutline_NonMarkdown(t *testing.T) {
	o := NewOutline("a.txt", resp("# x\n"), 4096)
	if o.Shown != 0 || !strings.Contains(o.Text(), "bukan file Markdown") || o.Summary()["count"] != 0 || o.Result().IsError {
		t.Fatalf("%+v %s", o, o.Text())
	}
	if o.Structured()["size_bytes"] != 4 {
		t.Fatalf("%v", o.Structured())
	}
}

func TestNewOutline_NoHeadings(t *testing.T) {
	o := NewOutline("a.md", resp("hanya teks\n"), 4096)
	if !strings.Contains(o.Text(), "Tidak ada heading Markdown di a.md") || o.Result().IsError {
		t.Fatalf("%s", o.Text())
	}
	if hs := o.Structured()["headings"].([]map[string]any); hs == nil || len(hs) != 0 {
		t.Fatalf("headings = %#v", hs)
	}
}

func TestNewOutline_BudgetTruncates(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&b, "## Heading nomor %d\nisi\n", i)
	}
	o := NewOutline("big.md", resp(b.String()), 4096)
	txt := o.Text()
	if len(txt) > 4096 {
		t.Fatalf("len(Text) = %d", len(txt))
	}
	if o.Shown == 0 || o.Shown >= 5000 || !strings.Contains(txt, "[outline terpotong:") {
		t.Fatalf("shown=%d\n%s", o.Shown, txt[len(txt)-300:])
	}
	if got := len(o.Structured()["headings"].([]map[string]any)); got != o.Shown {
		t.Fatalf("structured=%d shown=%d", got, o.Shown)
	}
	if o.Summary()["truncated"] != true || o.Summary()["total"] != 5000 {
		t.Fatalf("summary = %v", o.Summary())
	}
	want := fmt.Sprintf("menampilkan %d dari 5000 heading; sisanya mulai byte ", o.Shown)
	if !strings.Contains(txt, want) {
		t.Fatalf("marker salah:\n%s", txt[len(txt)-300:])
	}
}

func TestNewOutline_LongHeadingTextCapped(t *testing.T) {
	o := NewOutline("a.md", resp("# "+strings.Repeat("x", 5000)+"\n"), 2048)
	if len(o.Text()) > 2048 {
		t.Fatalf("len = %d", len(o.Text()))
	}
}

func TestOutline_SummaryHasNoContent(t *testing.T) {
	o := NewOutline("a.md", resp("# SECRET-XYZ\n"), 4096)
	b, _ := json.Marshal(o.Summary())
	if strings.Contains(string(b), "SECRET-XYZ") {
		t.Fatalf("bocor: %s", b)
	}
	for k, v := range o.Summary() {
		if _, ok := v.(string); ok {
			t.Fatalf("nilai string di %s", k)
		}
	}
}
