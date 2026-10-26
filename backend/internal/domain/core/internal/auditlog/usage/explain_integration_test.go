//go:build integration

package usage

import (
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

// TestT19_ExplainUsesIndex: query TopFiles/UnreadPaths/ZeroQueries/PerKey memakai indeks pada data besar
// dan selesai cepat. Empat query dijalankan nyata (bukan hanya EXPLAIN) dengan batas waktu.
func TestT19_ExplainUsesIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("seed 300 ribu baris dilewati dengan -short")
	}
	d := setupDB(t)
	if _, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, action, target, status, args, result)
		SELECT gen_random_uuid(), now() - (g * interval '2 seconds'), 'api_key',
		       md5('a' || (g % 50)::text)::uuid, 'key-' || (g % 50),
		       md5((g % 20)::text)::uuid,
		       (ARRAY['mcp.read_file','mcp.search_knowledge','mcp.list_files','auth.login.success','auth.logout','project.created','apikey.created','mcp.read_section'])[1 + g % 8],
		       'doc-' || (g % 500) || '.md', 'ok',
		       jsonb_build_object('query', 'q' || (g % 40)), jsonb_build_object('total', g % 3)
		FROM generate_series(1, 300000) g`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`ANALYZE audit_log`); err != nil {
		t.Fatal(err)
	}
	var pid string
	if err := d.QueryRow(`SELECT md5('3')::uuid::text`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	w := base.NewWindow(base.Range7d, now)
	r := NewRepository(d)

	start := time.Now()
	if _, err := r.TopFiles(ctx, w, pid, 20); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		paths = append(paths, "doc-"+itoa(i)+".md")
	}
	if _, err := r.UnreadPaths(ctx, w, pid, paths, 51); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ZeroQueries(ctx, w, "", 20); err != nil {
		t.Fatal(err)
	}
	if _, err := r.PerKey(ctx, w, "", 20); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 10*time.Second {
		t.Errorf("empat query terlalu lambat: %v", el)
	} else {
		t.Logf("empat query: %v", el)
	}

	// Pencocokan indeks pada predikat project (TopFiles) dan action (ZeroQueries).
	explain := func(q string, args ...any) string {
		var lines []string
		if err := d.Select(&lines, "EXPLAIN (FORMAT TEXT) "+q, args...); err != nil {
			t.Fatal(err)
		}
		return strings.Join(lines, "\n")
	}
	plan := explain(`SELECT target, count(*) FROM audit_log WHERE action = ANY($1::text[]) AND status='ok' AND project_id=$2::uuid AND target <> '' AND occurred_at >= $3 AND occurred_at < $4 GROUP BY target`,
		base.ReadActions(), pid, w.From, w.To)
	if !strings.Contains(plan, "audit_log_project_idx") && !strings.Contains(plan, "audit_log_action_idx") {
		t.Errorf("top files tidak memakai indeks:\n%s", plan)
	}
	plan = explain(`SELECT count(*) FROM audit_log WHERE action = $1 AND status='ok' AND occurred_at >= $2 AND occurred_at < $3`,
		"mcp.search_knowledge", w.From, w.To)
	if !strings.Contains(plan, "audit_log_action_idx") && !strings.Contains(plan, "audit_log_occurred_idx") {
		t.Errorf("zero queries tidak memakai indeks:\n%s", plan)
	}
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return string(b)
}
