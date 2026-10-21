package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"grimoire/lib/contracts/project/knowledge/listfiles"
)

var t0 = time.Date(2026, 10, 20, 8, 15, 0, 0, time.UTC)

func TestListPage_TextAndNext(t *testing.T) {
	resp := listfiles.Response{
		Items:   []listfiles.Item{{Path: "docs/a.md", SizeBytes: 1229, UpdatedAt: t0}, {Path: "docs/big.txt", SizeBytes: 204800, UpdatedAt: t0}},
		Total:   5,
		Folders: nil,
	}
	p := NewListPage("docs", true, 0, resp)
	want := "Folder: docs | rekursif: ya | 5 file\ndocs/a.md  1.2 KB  2026-10-20 08:15 UTC\ndocs/big.txt  200.0 KB  2026-10-20 08:15 UTC\n\n[ada 3 file lagi, panggil list_files dengan offset=2 path=docs recursive=true]"
	if got := p.Text(); got != want {
		t.Fatalf("got:\n%s", got)
	}
	if p.NextOffset() != 2 {
		t.Fatal(p.NextOffset())
	}
	last := NewListPage("", true, 4, listfiles.Response{Items: resp.Items[:1], Total: 5})
	if last.NextOffset() != -1 || strings.Contains(last.Text(), "[ada") {
		t.Fatal(last.Text())
	}
}

func TestListPage_FoldersOnlyFirstPageNonRecursive(t *testing.T) {
	resp := listfiles.Response{Items: []listfiles.Item{{Path: "readme.md", SizeBytes: 100, UpdatedAt: t0}}, Folders: []string{"docs/", "", "src"}, Total: 1}
	p := NewListPage("", false, 0, resp)
	if len(p.Folders) != 2 || p.Folders[0] != "docs" {
		t.Fatal(p.Folders)
	}
	if !strings.Contains(p.Text(), "[folder] docs/") || !strings.Contains(p.Text(), "Folder: / (akar) | rekursif: tidak | 1 file") {
		t.Fatal(p.Text())
	}
	if NewListPage("", true, 0, resp).Folders != nil || NewListPage("", false, 1, resp).Folders != nil {
		t.Fatal("folders harus nil")
	}
}

func TestListPage_Empty(t *testing.T) {
	r := NewListPage("", false, 0, listfiles.Response{}).Result()
	if r.IsError || r.Text != "Belum ada file knowledge di project ini." {
		t.Fatalf("%+v", r)
	}
	b, _ := json.Marshal(r.Structured)
	if string(b) != `{"folders":[],"items":[],"next_offset":null,"recursive":false,"total":0}` {
		t.Fatal(string(b))
	}
	r = NewListPage("docs", false, 0, listfiles.Response{}).Result()
	if r.IsError || !strings.Contains(r.Text, "Tidak ada file di folder 'docs'") {
		t.Fatal(r.Text)
	}
	// akar hanya berisi subfolder bukan kosong
	r = NewListPage("", false, 0, listfiles.Response{Folders: []string{"docs"}}).Result()
	if !strings.Contains(r.Text, "0 file") || !strings.Contains(r.Text, "[folder] docs/") {
		t.Fatal(r.Text)
	}
}

func TestListPage_OffsetOutOfRange(t *testing.T) {
	p := NewListPage("docs", false, 10, listfiles.Response{Total: 3})
	r := p.Result()
	if !p.OffsetOutOfRange() || !r.IsError || r.Structured != nil || r.Summary["error_code"] != "offset_out_of_range" ||
		r.Text != "Offset 10 melewati jumlah file (3). Gunakan offset 0-2." {
		t.Fatalf("%+v", r)
	}
}

func TestListPage_StructuredAndSummary(t *testing.T) {
	p := NewListPage("", true, 0, listfiles.Response{Items: []listfiles.Item{{Path: "a", SizeBytes: 3, UpdatedAt: t0}}, Total: 2})
	r := p.Result()
	b, _ := json.Marshal(r.Structured)
	if string(b) != `{"folders":[],"items":[{"path":"a","size_bytes":3,"updated_at":"2026-10-20T08:15:00Z"}],"next_offset":1,"recursive":true,"total":2}` {
		t.Fatal(string(b))
	}
	if r.TargetType != "folder" || r.Summary["count"] != 1 || r.Summary["next_offset"] != 1 || r.Summary["total"] != 2 {
		t.Fatalf("%+v", r.Summary)
	}
	if NewListPage("", true, 0, listfiles.Response{Total: 0}).Summary()["next_offset"] != -1 {
		t.Fatal("next_offset habis harus -1")
	}
}
