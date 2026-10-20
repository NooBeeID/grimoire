//go:build integration

package listentries

import (
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

func TestT02_AC9_ExplainUsesIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("seed 1 juta baris dilewati dengan -short")
	}
	d := setupDB(t)
	if _, err := d.Exec(`INSERT INTO audit_log (id, occurred_at, actor_type, actor_id, actor_label, project_id, action, status)
		SELECT gen_random_uuid(), now() - (g * interval '4 seconds'), 'api_key',
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

	now := time.Now().UTC()
	proj := "SELECT md5('3')::uuid::text"
	var projID, actorID string
	if err := d.QueryRow(proj).Scan(&projID); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT md5('a' || '7')::uuid::text`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	cur := base.Cursor{OccurredAt: now.Add(-24 * time.Hour), ID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}

	cases := []struct {
		name  string
		f     base.Filter
		limit int
	}{
		{"project+aksi+rentang", base.Filter{ProjectID: projID, Actions: []string{"mcp.read_file", "mcp.search_knowledge"}, From: now.Add(-7 * 24 * time.Hour)}, 51},
		{"actor", base.Filter{ActorType: "api_key", ActorID: actorID}, 51},
		{"tanpa filter", base.Filter{}, 51},
		{"cursor tanpa filter", base.Filter{Cursor: &cur}, 51},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sql, args := buildQuery(c.f, c.limit)
			var lines []string
			if err := d.Select(&lines, "EXPLAIN (FORMAT TEXT) "+sql, args...); err != nil {
				t.Fatal(err)
			}
			plan := strings.Join(lines, "\n")
			if strings.Contains(plan, "Seq Scan on audit_log") {
				t.Fatalf("seq scan:\n%s", plan)
			}
			if !strings.Contains(plan, "Index") {
				t.Fatalf("tanpa indeks:\n%s", plan)
			}
		})
	}
}
