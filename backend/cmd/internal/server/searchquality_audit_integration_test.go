//go:build integration

package server

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// sqaAuditRow adalah baris audit_log terbaru untuk action mcp.search_knowledge.
type sqaAuditRow struct {
	Target    string
	Status    string
	ProjectID string
	Args      map[string]any
	Result    map[string]any
	RawResult string
}

// sqaLastSearchAudit membaca baris terbaru action mcp.search_knowledge.
func sqaLastSearchAudit(t *testing.T, e *mcpEnv) sqaAuditRow {
	t.Helper()
	q := "SELECT coalesce(target,''), status, coalesce(project_id::text,''), args::text, result::text FROM " + e.schema +
		".audit_log WHERE action='mcp.search_knowledge' ORDER BY occurred_at DESC, id DESC LIMIT 1"
	var row sqaAuditRow
	var args string
	if err := e.admin.QueryRow(q).Scan(&row.Target, &row.Status, &row.ProjectID, &args, &row.RawResult); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(args), &row.Args); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(row.RawResult), &row.Result); err != nil {
		t.Fatal(err)
	}
	return row
}

// sqaWaitAudit menunggu jumlah audit search_knowledge mencapai want (audit tidak dijamin sinkron).
func sqaWaitAudit(t *testing.T, e *mcpEnv, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for e.countAudit("mcp.search_knowledge") < want {
		if time.Now().After(deadline) {
			t.Fatalf("audit mcp.search_knowledge: want %d, got %d", want, e.countAudit("mcp.search_knowledge"))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

var sqaTextRe = regexp.MustCompile(`(?m)^\d+\. (\S+)  \(skor`)

// sqaPathsFromText mengurai baris "N. path  (skor ...)" dari teks hasil.
func sqaPathsFromText(text string) []string {
	var out []string
	for _, m := range sqaTextRe.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

// sqaPathsFromResult mengubah result["result_paths"] menjadi []string; fatal bila bukan array string.
func sqaPathsFromResult(t *testing.T, res map[string]any) []string {
	t.Helper()
	if res["result_paths"] == nil {
		t.Fatalf("result_paths tidak ada/null: %v", res)
	}
	arr, ok := res["result_paths"].([]any)
	if !ok {
		t.Fatalf("result_paths bukan array: %#v", res["result_paths"])
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("elemen bukan string: %#v", v)
		}
		out = append(out, s)
	}
	return out
}

func sqaStructuredPaths(t *testing.T, r ktResult) []string {
	t.Helper()
	var out []string
	for _, it := range ktItems(t, r) {
		out = append(out, it["path"].(string))
	}
	return out
}

// sqaSeed menyiapkan n file "kb/doc%02d.md" yang semuanya memuat kata "kuota" (isi unik per file).
func sqaSeed(e *mcpEnv, n int) {
	for i := 1; i <= n; i++ {
		e.seedKnowledge(e.projID, fmt.Sprintf("kb/doc%02d.md", i), fmt.Sprintf("# Doc %d\nkuota nomor %d untuk pengujian audit", i, i))
	}
}

func TestT11_Int_AC1_ResultPathsInAudit(t *testing.T) {
	e := setupMCP(t)
	sqaSeed(e, 3)
	r := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "kuota"})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	sqaWaitAudit(t, e, 1)
	row := sqaLastSearchAudit(t, e)
	if row.Status != "ok" {
		t.Fatalf("status: %s", row.Status)
	}
	got := sqaPathsFromResult(t, row.Result)
	if len(got) != 3 {
		t.Fatalf("result_paths: %v", got)
	}
	if want := sqaPathsFromText(r.Text); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit %v != teks %v", got, want)
	}
	if want := sqaStructuredPaths(t, r); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit %v != structured %v", got, want)
	}
	if row.Result["count"] != float64(3) || row.Result["total"] != float64(3) {
		t.Fatalf("count/total: %v", row.Result)
	}
	if _, has := row.Result["result_paths_truncated"]; has {
		t.Fatalf("flag tidak boleh ada: %v", row.Result)
	}
	for _, k := range []string{"bytes", "duration_ms"} {
		if _, ok := row.Result[k]; !ok {
			t.Fatalf("kunci gateway %s hilang: %v", k, row.Result)
		}
	}
	if _, has := row.Result["_truncated"]; has {
		t.Fatalf("result diganti Sanitize: %v", row.Result)
	}
	if row.Args["query"] != "kuota" {
		t.Fatalf("args: %v", row.Args)
	}
	for _, bad := range []string{"snippet", "score"} {
		if strings.Contains(row.RawResult, bad) {
			t.Fatalf("result memuat %q: %s", bad, row.RawResult)
		}
	}
}

func TestT11_Int_AC2_CappedTenWithRealTotal(t *testing.T) {
	e := setupMCP(t)
	sqaSeed(e, 15)
	r := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "kuota", "limit": 15})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	fromText := sqaPathsFromText(r.Text)
	if len(fromText) != 15 || len(ktItems(t, r)) != 15 {
		t.Fatalf("agent harus menerima 15 hasil: %d / %d", len(fromText), len(ktItems(t, r)))
	}
	sqaWaitAudit(t, e, 1)
	row := sqaLastSearchAudit(t, e)
	got := sqaPathsFromResult(t, row.Result)
	if len(got) != 10 || !reflect.DeepEqual(got, fromText[:10]) {
		t.Fatalf("result_paths: %v", got)
	}
	if row.Result["count"] != float64(15) || row.Result["total"] != float64(15) {
		t.Fatalf("count/total: %v", row.Result)
	}
}

func TestT11_Int_AC4_NoResultsEmptyArray(t *testing.T) {
	e := setupMCP(t)
	sqaSeed(e, 2)
	r := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "zzzzzzzz-tidak-ada"})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	sqaWaitAudit(t, e, 1)
	row := sqaLastSearchAudit(t, e)
	if got := sqaPathsFromResult(t, row.Result); len(got) != 0 {
		t.Fatalf("result_paths: %v", got)
	}
	if row.Result["total"] != float64(0) || row.Result["count"] != float64(0) {
		t.Fatalf("%v", row.Result)
	}
	if !strings.Contains(row.RawResult, `"result_paths": []`) {
		t.Fatalf("raw: %s", row.RawResult)
	}
}

func TestT11_Int_AC3_NoContentInAudit(t *testing.T) {
	e := setupMCP(t)
	e.seedKnowledge(e.projID, "kb/rahasia.md", "RAHASIA-ISI-UNIK adalah kalimat yang hanya ada di isi file")
	r := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "RAHASIA-ISI-UNIK"})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	sqaWaitAudit(t, e, 1)
	row := sqaLastSearchAudit(t, e)
	if got := sqaPathsFromResult(t, row.Result); !reflect.DeepEqual(got, []string{"kb/rahasia.md"}) {
		t.Fatalf("result_paths: %v", got)
	}
	if strings.Contains(strings.ToLower(row.RawResult), "rahasia-isi-unik") || strings.Contains(row.RawResult, "kalimat") {
		t.Fatalf("result membocorkan isi/query: %s", row.RawResult)
	}
	for _, k := range []string{"snippet", "score", "content", "query"} {
		if _, ok := row.Result[k]; ok {
			t.Fatalf("kunci terlarang %s: %v", k, row.Result)
		}
	}
	if row.Args["query"] != "RAHASIA-ISI-UNIK" {
		t.Fatalf("args: %v", row.Args)
	}
}

func TestT11_Int_AC6_AuditFailureDoesNotChangeAgentResponse(t *testing.T) {
	e := setupMCP(t)
	sqaSeed(e, 3)
	args := map[string]any{"query": "kuota"}
	ctrl := e.mcpCall(e.rawKey, "search_knowledge", args)
	if ctrl.IsError {
		t.Fatalf("kontrol: %s", ctrl.Raw)
	}
	sqaWaitAudit(t, e, 1)
	before := e.countAudit("mcp.search_knowledge")
	if before != 1 {
		t.Fatalf("before=%d", before)
	}

	// Registry bus tidak punya override handler (Register panic pada duplikat), jadi core.audit.record
	// dirusak dengan mengganti nama tabel di skema tes terisolasi; INSERT gagal dengan error SQL nyata.
	renamed := false
	restore := func() {
		if renamed {
			_, _ = e.admin.Exec("ALTER TABLE " + e.schema + ".audit_log_sqa_broken RENAME TO audit_log")
			renamed = false
		}
	}
	t.Cleanup(restore)
	if _, err := e.admin.Exec("ALTER TABLE " + e.schema + ".audit_log RENAME TO audit_log_sqa_broken"); err != nil {
		t.Fatal(err)
	}
	renamed = true

	broken := e.mcpCall(e.rawKey, "search_knowledge", args)
	if broken.IsError {
		t.Fatalf("audit gagal mengubah isError: %s", broken.Raw)
	}
	if broken.Text != ctrl.Text {
		t.Fatalf("teks berbeda:\n%s\n---\n%s", broken.Text, ctrl.Text)
	}
	if !reflect.DeepEqual(broken.StructuredContent, ctrl.StructuredContent) {
		t.Fatalf("structuredContent berbeda: %v vs %v", broken.StructuredContent, ctrl.StructuredContent)
	}
	if !reflect.DeepEqual(broken.Meta, ctrl.Meta) {
		t.Fatalf("_meta berbeda: %v vs %v", broken.Meta, ctrl.Meta)
	}
	// Tanpa hasil juga tetap sukses saat audit rusak.
	empty := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "zzzzzzzz-tidak-ada"})
	if empty.IsError {
		t.Fatalf("query kosong saat audit rusak: %s", empty.Raw)
	}

	restore()
	if n := e.countAudit("mcp.search_knowledge"); n != before {
		t.Fatalf("panggilan saat rusak tercatat: %d != %d", n, before)
	}
	again := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "nomor"})
	if again.IsError {
		t.Fatalf("setelah pulih: %s", again.Raw)
	}
	sqaWaitAudit(t, e, before+1)
	row := sqaLastSearchAudit(t, e)
	if got := sqaPathsFromResult(t, row.Result); !reflect.DeepEqual(got, sqaPathsFromText(again.Text)) || len(got) == 0 {
		t.Fatalf("result_paths setelah pulih: %v", got)
	}
}

func TestT11_Int_ProjectIsolation(t *testing.T) {
	e := setupMCP(t)
	sqaSeed(e, 2)
	projB := e.newProject("Other")
	e.seedKnowledge(projB, "secret-b/kuota.md", "kuota rahasia project B")
	r := e.mcpCall(e.rawKey, "search_knowledge", map[string]any{"query": "kuota"})
	if r.IsError {
		t.Fatalf("isError: %s", r.Raw)
	}
	sqaWaitAudit(t, e, 1)
	row := sqaLastSearchAudit(t, e)
	got := sqaPathsFromResult(t, row.Result)
	if len(got) != 2 {
		t.Fatalf("result_paths: %v", got)
	}
	for _, p := range got {
		if strings.Contains(p, "secret-b") {
			t.Fatalf("path project lain bocor: %v", got)
		}
	}
	if row.ProjectID != e.projID {
		t.Fatalf("project_id %s != %s", row.ProjectID, e.projID)
	}
}
