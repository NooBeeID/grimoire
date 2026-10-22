//go:build integration

package summary

import (
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

func TestT15_ExplainUsesIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("seed 1 juta baris dilewati dengan -short")
	}
	d := setupDB(t)
	if _, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, action, status)
		SELECT gen_random_uuid(), now() - (g * interval '7.776 seconds'), 'api_key',
		       md5('a' || (g % 50)::text)::uuid, 'key-' || (g % 50),
		       md5((g % 20)::text)::uuid,
		       (ARRAY['mcp.read_file','mcp.search_knowledge','mcp.list_files','auth.login.success','auth.logout','project.created','apikey.created','mcp.rate_limited'])[1 + g % 8],
		       (ARRAY['ok','error','denied'])[1 + g % 3]
		FROM generate_series(1, 1000000) g`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`ANALYZE audit_log`); err != nil {
		t.Fatal(err)
	}
	var projID string
	if err := d.QueryRow(`SELECT md5('3')::uuid::text`).Scan(&projID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	explain := func(w base.Window, pid string, strict bool, label string) {
		where, args := buildWhere(w, pid)
		qs := map[string]string{
			"tools":    "SELECT action, count(*) FROM audit_log WHERE " + where + " GROUP BY action",
			"projects": "SELECT project_id, count(*) FROM audit_log WHERE " + where + " AND project_id IS NOT NULL GROUP BY project_id",
			"traffic":  "SELECT date_trunc('" + string(w.Bucket) + "', occurred_at, 'UTC'), count(*) FROM audit_log WHERE " + where + " GROUP BY 1",
		}
		for name, q := range qs {
			var lines []string
			if err := d.Select(&lines, "EXPLAIN (FORMAT TEXT) "+q, args...); err != nil {
				t.Fatal(err)
			}
			plan := strings.Join(lines, "\n")
			if strict {
				if !strings.Contains(plan, "audit_log_action_idx") && !strings.Contains(plan, "audit_log_project_idx") {
					t.Errorf("%s/%s: tidak memakai indeks:\n%s", label, name, plan)
				}
				if strings.Contains(plan, "Seq Scan on audit_log") {
					t.Errorf("%s/%s: Seq Scan:\n%s", label, name, plan)
				}
			}
		}
	}
	explain(base.NewWindow(base.Range24h, now), "", true, "24h")
	explain(base.NewWindow(base.Range30d, now), projID, true, "30d+project")

	// 30d tanpa project: tidak di-assert, hanya dicatat untuk deskripsi PR.
	w := base.NewWindow(base.Range30d, now)
	start := time.Now()
	if _, err := NewRepository(d).Load(t.Context(), w, ""); err != nil {
		t.Fatal(err)
	}
	t.Logf("Load 30d tanpa project pada 1 juta baris: %s", time.Since(start))
}
