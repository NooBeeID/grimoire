package searchknowledge

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/knowledge/search"
)

// items membangun n search.Item dengan path "docs/f%02d.md", skor menurun, snippet "sn %d".
func items(n int) []search.Item {
	out := make([]search.Item, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, search.Item{Path: fmt.Sprintf("docs/f%02d.md", i), Score: 1 - float64(i)/100, Snippet: fmt.Sprintf("sn %d", i)})
	}
	return out
}

// resultPaths mengambil Summary["result_paths"] sebagai []string; fatal bila bukan []string.
func resultPaths(t *testing.T, s map[string]any) []string {
	t.Helper()
	v, ok := s["result_paths"].([]string)
	if !ok {
		t.Fatalf("result_paths bukan []string: %#v", s["result_paths"])
	}
	return v
}

func summaryFor(t *testing.T, resp search.Response, args string) map[string]any {
	t.Helper()
	f := newFakeBus(true)
	f.resp = resp
	r, err := f.call("p1", args)
	if err != nil || r.IsError {
		t.Fatalf("%+v %v", r, err)
	}
	return r.Summary
}

func TestSummary_AC1_ThreePathsInOrder(t *testing.T) {
	f := newFakeBus(true)
	f.resp = search.Response{Items: items(3), Total: 3}
	r, err := f.call("p1", `{"query":"q"}`)
	if err != nil || r.IsError {
		t.Fatalf("%+v %v", r, err)
	}
	want := []string{"docs/f01.md", "docs/f02.md", "docs/f03.md"}
	got := resultPaths(t, r.Summary)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result_paths: %v want %v", got, want)
	}
	re := regexp.MustCompile(`(?m)^\d+\. (\S+)  \(skor`)
	var fromText []string
	for _, m := range re.FindAllStringSubmatch(r.Text, -1) {
		fromText = append(fromText, m[1])
	}
	if !reflect.DeepEqual(fromText, want) {
		t.Fatalf("urutan teks: %v want %v", fromText, want)
	}
	var fromStruct []string
	for _, it := range r.Structured["items"].([]map[string]any) {
		fromStruct = append(fromStruct, it["path"].(string))
	}
	if !reflect.DeepEqual(fromStruct, want) {
		t.Fatalf("urutan structured: %v want %v", fromStruct, want)
	}
	if r.Summary["count"] != 3 || r.Summary["total"] != 3 {
		t.Fatalf("count/total: %v", r.Summary)
	}
	if _, has := r.Summary["result_paths_truncated"]; has {
		t.Fatalf("flag tidak boleh ada: %v", r.Summary)
	}
}

func TestSummary_AC2_CappedAtTen(t *testing.T) {
	for _, total := range []int{15, 42} {
		s := summaryFor(t, search.Response{Items: items(15), Total: total}, `{"query":"q","limit":20}`)
		got := resultPaths(t, s)
		if len(got) != 10 {
			t.Fatalf("total=%d len=%d", total, len(got))
		}
		for i, p := range got {
			if p != fmt.Sprintf("docs/f%02d.md", i+1) {
				t.Fatalf("path[%d]=%s", i, p)
			}
		}
		if s["count"] != 15 || s["total"] != total {
			t.Fatalf("total=%d summary=%v", total, s)
		}
		if _, has := s["result_paths_truncated"]; has {
			t.Fatalf("batas 10 bukan truncation anggaran: %v", s)
		}
	}
}

func TestSummary_AC4_NoResultsEmptyArray(t *testing.T) {
	s := summaryFor(t, search.Response{}, `{"query":"q"}`)
	got := resultPaths(t, s)
	if got == nil || len(got) != 0 {
		t.Fatalf("harus slice kosong non-nil: %#v", got)
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"result_paths":[]`) || strings.Contains(string(b), `"result_paths":null`) {
		t.Fatalf("json: %s", b)
	}
	if s["total"] != 0 || s["count"] != 0 {
		t.Fatalf("%v", s)
	}
}

func TestSummary_AC3_NoLeakedKeys(t *testing.T) {
	its := []search.Item{
		{Path: "docs/a.md", Score: 0.6079, Snippet: "**rate** a"},
		{Path: "docs/b.md", Score: 0.5, Snippet: "**rate** b"},
		{Path: "docs/c.md", Score: 0.4, Snippet: "**rate** c"},
	}
	s := summaryFor(t, search.Response{Items: its, Total: 3}, `{"query":"rahasia-query","path":"docs"}`)
	if len(s) != 3 {
		t.Fatalf("kunci Summary harus count,total,result_paths: %v", s)
	}
	for _, k := range []string{"count", "total", "result_paths"} {
		if _, ok := s[k]; !ok {
			t.Fatalf("kunci %s hilang: %v", k, s)
		}
	}
	b, _ := json.Marshal(s)
	for _, bad := range []string{"snippet", "score", "content", "query", "rahasia-query", "**rate**", "0.6079"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("summary membocorkan %q: %s", bad, b)
		}
	}
}

func TestSummary_LongPathTruncatedAtUTF8(t *testing.T) {
	for _, ch := range []string{"é", "日"} {
		orig := "docs/" + strings.Repeat(ch, 800)
		s := summaryFor(t, search.Response{Items: []search.Item{{Path: orig, Score: 1}}, Total: 1}, `{"query":"q"}`)
		got := resultPaths(t, s)
		if len(got) != 1 {
			t.Fatalf("len=%d", len(got))
		}
		p := got[0]
		if len(p) > record.MaxTargetBytes || !utf8.ValidString(p) || !strings.HasPrefix(orig, p) {
			t.Fatalf("%s: len=%d valid=%v", ch, len(p), utf8.ValidString(p))
		}
		// byte terbesar <= 1024 pada batas rune: rune berikutnya tidak lagi muat.
		_, size := utf8.DecodeRuneInString(orig[len(p):])
		if len(p)+size <= record.MaxTargetBytes {
			t.Fatalf("%s: dipotong terlalu pendek (len=%d)", ch, len(p))
		}
		if s["count"] != 1 || s["total"] != 1 {
			t.Fatalf("%v", s)
		}
	}
}

func TestSummary_BudgetTruncationFlag(t *testing.T) {
	its := make([]search.Item, 10)
	for i := range its {
		// tiap path tepat 1024 byte; unik agar urutan dapat dibedakan
		its[i] = search.Item{Path: fmt.Sprintf("docs/%02d", i) + strings.Repeat("a", 1024-7), Score: 1}
	}
	s := summaryFor(t, search.Response{Items: its, Total: 10}, `{"query":"q"}`)
	got := resultPaths(t, s)
	if s["result_paths_truncated"] != true {
		t.Fatalf("flag hilang: count=%d", len(got))
	}
	// Rumus: 2 + 1026 + n*1027 untuk n tambahan; 6 path = 6163 > 6144, jadi 5 path (5136 byte).
	if len(got) != 5 {
		t.Fatalf("len=%d want 5", len(got))
	}
	size := 2 + len(got) - 1
	for _, p := range got {
		b, _ := json.Marshal(p)
		size += len(b)
	}
	if size > base.ResultPathsBudgetBytes {
		t.Fatalf("anggaran terlampaui: %d", size)
	}
	// Simulasi gateway: kunci gateway ditambahkan, total harus di bawah batas Sanitize.
	merged := map[string]any{"bytes": 99999, "original_bytes": 99999, "truncated": false, "duration_ms": 99999, "error_code": "invalid_request"}
	for k, v := range s {
		merged[k] = v
	}
	b, _ := json.Marshal(merged)
	if len(b) >= record.MaxResultJSONBytes {
		t.Fatalf("result %d byte >= %d", len(b), record.MaxResultJSONBytes)
	}
	if _, has := merged["_truncated"]; has {
		t.Fatal("_truncated tidak boleh muncul")
	}
}

func TestSummary_BudgetNoFlagWhenOnlyCapTen(t *testing.T) {
	s := summaryFor(t, search.Response{Items: items(15), Total: 15}, `{"query":"q"}`)
	if _, has := s["result_paths_truncated"]; has {
		t.Fatalf("%v", s)
	}
}

func TestSummary_EscapedCharsCountedByEncodedLength(t *testing.T) {
	its := make([]search.Item, 10)
	for i := range its {
		its[i] = search.Item{Path: fmt.Sprintf("docs/%d", i) + strings.Repeat("<", 149), Score: 1}
	}
	s := summaryFor(t, search.Response{Items: its, Total: 10}, `{"query":"q"}`)
	got := resultPaths(t, s)
	b, _ := json.Marshal(got)
	if len(b) > base.ResultPathsBudgetBytes {
		t.Fatalf("json result_paths %d > %d", len(b), base.ResultPathsBudgetBytes)
	}
	if len(got) >= 10 {
		t.Fatalf("path ter-escape harus memicu pembuangan, len=%d", len(got))
	}
	if s["result_paths_truncated"] != true {
		t.Fatalf("flag hilang: %v", s)
	}
}

func TestSummary_PathOrderPreservedIncludingDuplicatePrefix(t *testing.T) {
	its := []search.Item{{Path: "z.md"}, {Path: "a.md"}, {Path: "m.md"}, {Path: "a.md"}}
	s := summaryFor(t, search.Response{Items: its, Total: 4}, `{"query":"q"}`)
	want := []string{"z.md", "a.md", "m.md", "a.md"}
	if got := resultPaths(t, s); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}

func TestSummary_SpecialQueryAndPrefixNotInSummary(t *testing.T) {
	s := summaryFor(t, search.Response{Items: items(1), Total: 1}, `{"query":"\"a\" <b> & c","path":"docs/guides"}`)
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "guides") || strings.Contains(string(b), "<b>") || len(s) != 3 {
		t.Fatalf("%s", b)
	}
}

func TestSummary_ErrorResultHasNoResultPaths(t *testing.T) {
	f := newFakeBus(true)
	r, err := f.call("p1", `{"query":""}`)
	if err != nil || !r.IsError {
		t.Fatalf("%+v %v", r, err)
	}
	if _, has := r.Summary["result_paths"]; has {
		t.Fatalf("%v", r.Summary)
	}
}
