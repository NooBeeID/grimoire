package base

import (
	"testing"
	"time"
)

func metas(paths ...string) []Meta {
	var out []Meta
	for _, p := range paths {
		out = append(out, Meta{Path: p, SizeBytes: len(p), UpdatedAt: time.Unix(1, 0).UTC()})
	}
	return out
}

func TestBuildLevel_Prefix(t *testing.T) {
	lv := BuildLevel("docs", metas("docs/a.md", "docs/auth/login.md", "docs/auth/flow/x.md", "docs2/y.md", "other.md"))
	if lv.Prefix != "docs" || lv.TotalFiles != 3 {
		t.Fatalf("%+v", lv)
	}
	if len(lv.Folders) != 1 || lv.Folders[0] != (FolderNode{Name: "auth", Path: "docs/auth", FileCount: 2}) {
		t.Fatalf("%+v", lv.Folders)
	}
	if len(lv.Files) != 1 || lv.Files[0].Name != "a.md" || lv.Files[0].Path != "docs/a.md" || lv.Files[0].SizeBytes != 9 {
		t.Fatalf("%+v", lv.Files)
	}
}

func TestBuildLevel_RootAndOrder(t *testing.T) {
	lv := BuildLevel("", metas("z.md", "b/1.md", "a/2.md", "a/3.md", "c.md"))
	if lv.TotalFiles != 5 || len(lv.Folders) != 2 || lv.Folders[0].Name != "a" || lv.Folders[0].FileCount != 2 ||
		lv.Folders[1].Name != "b" || lv.Files[0].Name != "c.md" || lv.Files[1].Name != "z.md" {
		t.Fatalf("%+v", lv)
	}
}

func TestBuildLevel_EmptyNonNil(t *testing.T) {
	lv := BuildLevel("nothing", nil)
	if lv.Folders == nil || lv.Files == nil || lv.TotalFiles != 0 {
		t.Fatalf("%+v", lv)
	}
}
