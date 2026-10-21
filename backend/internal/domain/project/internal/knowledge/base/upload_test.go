package base

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestClassifyPart(t *testing.T) {
	const max = 1024
	cases := []struct {
		name     string
		rawName  string
		raw      string
		tooLarge bool
		status   Status
		reason   Reason
	}{
		{"ok", "docs/a.md", "# A", false, "", ""},
		{"hidden dir skipped", ".git/x.md", "x", false, StatusSkipped, ReasonHidden},
		{"ds store skipped", "a/.DS_Store", "x", false, StatusSkipped, ReasonHidden},
		{"png rejected", "x.png", "x", false, StatusRejected, ReasonUnsupportedType},
		{"traversal", "../x.md", "x", false, StatusRejected, ReasonPathTraversal},
		{"too large", "big.md", "x", true, StatusRejected, ReasonTooLarge},
		{"binary md", "bin.md", "\x00\x01", false, StatusRejected, ReasonInvalidEncoding},
		{"empty name", "", "x", false, StatusRejected, ReasonPathEmpty},
		{"content over max", "c.md", strings.Repeat("a", max+1), false, StatusRejected, ReasonTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, d, ok := ClassifyPart(7, tc.rawName, []byte(tc.raw), tc.tooLarge, max)
			if tc.status == "" {
				if !ok || in.Path != tc.rawName || in.Index != 7 || in.Content != tc.raw || len(in.Hash) != 32 {
					t.Fatalf("ok=%v in=%+v", ok, in)
				}
				return
			}
			if ok || d.Status != tc.status || d.Reason != tc.reason || d.Index != 7 {
				t.Fatalf("ok=%v d=%+v", ok, d)
			}
		})
	}
}

func TestClassifyPart_LongRawNameTruncated(t *testing.T) {
	_, d, _ := ClassifyPart(0, strings.Repeat("a", 600), nil, false, 10)
	if d.Reason != ReasonPathTooLong || len([]rune(d.Path)) != 600 {
		t.Fatalf("%q len=%d", d.Reason, len([]rune(d.Path)))
	}
	_, d, _ = ClassifyPart(0, strings.Repeat("a", 2000), nil, false, 10)
	if len([]rune(d.Path)) != 1024 {
		t.Fatalf("len=%d", len([]rune(d.Path)))
	}
}

func inc(i int, p, c string) Incoming { return Incoming{Index: i, Path: p, Content: c, Hash: Hash(c)} }

func TestPlanUpload(t *testing.T) {
	existing := map[string][]byte{"same.md": Hash("s"), "diff.md": Hash("old")}
	ds := PlanUpload([]Incoming{
		inc(0, "new.md", "n"),
		inc(1, "same.md", "s"),
		inc(2, "diff.md", "new"),
		inc(3, "new.md", "n2"),
	}, existing, 2, 100)
	want := []struct {
		s Status
		r Reason
	}{{StatusCreated, ""}, {StatusUnchanged, ""}, {StatusUpdated, ""}, {StatusRejected, ReasonDuplicateInRequest}}
	for i, w := range want {
		if ds[i].Status != w.s || ds[i].Reason != w.r || ds[i].Index != i {
			t.Errorf("ds[%d] = %+v, want %v/%v", i, ds[i], w.s, w.r)
		}
	}
}

func TestPlanUpload_Quota(t *testing.T) {
	ds := PlanUpload([]Incoming{inc(0, "a.md", "a"), inc(1, "b.md", "b"), inc(2, "c.md", "c")}, nil, 4998, 5000)
	if ds[0].Status != StatusCreated || ds[1].Status != StatusCreated ||
		ds[2].Status != StatusRejected || ds[2].Reason != ReasonProjectFileLimit {
		t.Fatalf("%+v", ds)
	}
	// update tidak memakan kuota walau project penuh
	ds = PlanUpload([]Incoming{inc(0, "x.md", "new"), inc(1, "y.md", "y")}, map[string][]byte{"x.md": Hash("old")}, 5, 5)
	if ds[0].Status != StatusUpdated || ds[1].Reason != ReasonProjectFileLimit {
		t.Fatalf("%+v", ds)
	}
}

func TestPlanUpload_OrdersByIndex(t *testing.T) {
	ds := PlanUpload([]Incoming{inc(5, "a.md", "1"), inc(2, "a.md", "2")}, nil, 0, 10)
	if ds[0].Index != 2 || ds[0].Status != StatusCreated || ds[1].Status != StatusRejected {
		t.Fatalf("first occurrence by index must win: %+v", ds)
	}
}

func TestSummarizeChangedPathsSort(t *testing.T) {
	var ds []Decision
	for i := 24; i >= 0; i-- {
		ds = append(ds, Decision{Index: i, Path: fmt.Sprintf("f%02d.md", i), Status: StatusCreated})
	}
	ds = append(ds,
		Decision{Index: 100, Status: StatusUpdated, Path: "u.md"},
		Decision{Index: 101, Status: StatusUnchanged},
		Decision{Index: 102, Status: StatusSkipped},
		Decision{Index: 103, Status: StatusRejected},
	)
	s := Summarize(ds)
	if s != (Summary{Created: 25, Updated: 1, Unchanged: 1, Skipped: 1, Rejected: 1}) {
		t.Fatalf("%+v", s)
	}
	paths, trunc := ChangedPaths(ds, 20)
	if len(paths) != 20 || !trunc || paths[0] != "f00.md" || paths[19] != "f19.md" {
		t.Fatalf("%v %v", paths, trunc)
	}
	paths, trunc = ChangedPaths(ds[:3], 20)
	if len(paths) != 3 || trunc {
		t.Fatalf("%v %v", paths, trunc)
	}
	if p, tr := ChangedPaths(nil, 5); p == nil || len(p) != 0 || tr {
		t.Fatal("empty must be non-nil")
	}
	SortByIndex(ds)
	if ds[0].Index != 0 || ds[len(ds)-1].Index != 103 {
		t.Fatal("SortByIndex")
	}
}

func TestNewUploadResult(t *testing.T) {
	r := NewUploadResult([]Decision{
		{Index: 1, Path: "b", Status: StatusRejected, Reason: ReasonHidden},
		{Index: 0, Path: "a", Status: StatusCreated},
	})
	if len(r.Items) != 2 || r.Items[0].Path != "a" || r.Summary.Created != 1 || r.Summary.Rejected != 1 {
		t.Fatalf("%+v", r)
	}
	if e := NewUploadResult(nil); e.Items == nil {
		t.Fatal("items must be non-nil")
	}
	if !bytes.Equal(Hash("x"), Hash("x")) {
		t.Fatal("unreachable")
	}
}
