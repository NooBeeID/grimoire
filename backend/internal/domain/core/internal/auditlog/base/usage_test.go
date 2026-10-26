package base

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUsageQuery_Validate(t *testing.T) {
	q := UsageQuery{}
	if err := q.Validate(); err != nil || q.Range != Range7d {
		t.Fatalf("range=%q err=%v", q.Range, err)
	}
	q = UsageQuery{Section: SectionKeys}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	q = UsageQuery{Section: "files"}
	if f := fieldOf(t, q.Validate()); f != "section" {
		t.Fatalf("field=%s", f)
	}
	q = UsageQuery{Range: "1y"}
	if f := fieldOf(t, q.Validate()); f != "range" {
		t.Fatalf("field=%s", f)
	}
	q = UsageQuery{ProjectID: "x"}
	if f := fieldOf(t, q.Validate()); f != "project_id" {
		t.Fatalf("field=%s", f)
	}
	id := "0A0B0C0D-0000-4000-8000-000000000001"
	q = UsageQuery{ProjectID: id}
	if err := q.Validate(); err != nil || q.ProjectID != strings.ToLower(id) {
		t.Fatalf("pid=%q err=%v", q.ProjectID, err)
	}
}

func TestUsageQuery_Wants(t *testing.T) {
	all := []Section{SectionTopFiles, SectionUnreadFiles, SectionZeroQueries, SectionKeys}
	q := UsageQuery{}
	for _, s := range all {
		if !q.Wants(s) {
			t.Fatalf("empty section should want %s", s)
		}
	}
	q = UsageQuery{Section: SectionKeys}
	for _, s := range all {
		if q.Wants(s) != (s == SectionKeys) {
			t.Fatalf("keys query Wants(%s)=%v", s, q.Wants(s))
		}
	}
}

func TestSection_NeedsProject(t *testing.T) {
	want := map[Section]bool{SectionTopFiles: true, SectionUnreadFiles: true, SectionZeroQueries: false, SectionKeys: false}
	for s, w := range want {
		if s.NeedsProject() != w {
			t.Fatalf("%s", s)
		}
	}
}

func TestReadActions(t *testing.T) {
	if !slices.Equal(ReadActions(), []string{"mcp.read_file", "mcp.read_section"}) {
		t.Fatalf("%v", ReadActions())
	}
}

func TestT19_AC2_BuildUnreadExcludesDeleted(t *testing.T) {
	ub := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	files := []KnownFile{{Path: "a.md", UpdatedAt: ub.Add(-time.Hour)}, {Path: "b.md", UpdatedAt: ub}}
	s := BuildUnread(files, []string{"b.md", "old.md"}, false)
	if len(s.Items) != 1 || s.Items[0].Path != "b.md" || !s.Items[0].UpdatedAt.Equal(ub) || s.Truncated || !s.Available {
		t.Fatalf("%+v", s)
	}
}

func TestBuildUnread_Truncation(t *testing.T) {
	var files []KnownFile
	var paths []string
	for i := 0; i < 51; i++ {
		p := fmt.Sprintf("f%02d.md", i)
		files = append(files, KnownFile{Path: p})
		paths = append(paths, p)
	}
	s := BuildUnread(files, paths, false)
	if len(s.Items) != UnreadLimit || !s.Truncated {
		t.Fatalf("len=%d trunc=%v", len(s.Items), s.Truncated)
	}
	s = BuildUnread(files, paths[:3], true)
	if len(s.Items) != 3 || !s.Truncated {
		t.Fatalf("len=%d trunc=%v", len(s.Items), s.Truncated)
	}
}

func TestBuildUnread_EmptyIsArray(t *testing.T) {
	b, _ := json.Marshal(BuildUnread(nil, nil, false))
	if !strings.Contains(string(b), `"items":[]`) {
		t.Fatal(string(b))
	}
}

func TestPerKeyOK_TopToolName(t *testing.T) {
	s := PerKeyOK([]KeyUsage{{KeyID: "k", TopTool: "mcp.read_file"}})
	if s.Items[0].TopTool != "read_file" {
		t.Fatal(s.Items[0].TopTool)
	}
}

func TestSectionsLimit(t *testing.T) {
	if n := len(TopFilesOK(make([]FileRead, 25)).Items); n != TopFilesLimit {
		t.Fatal(n)
	}
	if ZeroQueriesOK(nil).Items == nil {
		t.Fatal("nil items")
	}
	for _, b := range []any{TopFilesError("X"), UnreadFilesError("X"), ZeroQueriesError("X"), PerKeyError("X")} {
		j, _ := json.Marshal(b)
		if !strings.Contains(string(j), `"available":false`) || !strings.Contains(string(j), `"items":[]`) {
			t.Fatal(string(j))
		}
	}
}

func TestNewUsageReport_JSONShape(t *testing.T) {
	w := NewWindow(Range7d, time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC))
	b, err := json.Marshal(NewUsageReport(w, UsageQuery{}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"range", "from", "to", "project_id", "top_files", "unread_files", "zero_queries", "per_key"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	for _, k := range []string{"project_id", "top_files", "unread_files", "zero_queries", "per_key"} {
		if m[k] != nil {
			t.Fatalf("%s should be null", k)
		}
	}
}
