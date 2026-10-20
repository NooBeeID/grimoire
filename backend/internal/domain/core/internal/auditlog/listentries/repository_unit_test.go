package listentries

import (
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

func TestBuildQuery_NoFilter(t *testing.T) {
	sql, args := buildQuery(base.Filter{}, 51)
	if strings.Contains(sql, "WHERE") || !strings.HasSuffix(sql, "ORDER BY occurred_at DESC, id DESC LIMIT $1") {
		t.Fatal(sql)
	}
	if len(args) != 1 || args[0] != 51 {
		t.Fatal(args)
	}
}

func TestBuildQuery_AllFiltersParameterized(t *testing.T) {
	evil := "x'; DROP TABLE audit_log; --"
	f := base.Filter{
		ActorType: "api_key", ActorID: "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c",
		ActorLabel: evil, ProjectID: "0192f0c4-0000-7000-8000-000000000001",
		Actions: []string{"a.b", "c.d"}, Statuses: []string{"ok"},
		From: time.Now().Add(-time.Hour), To: time.Now(), Target: evil,
		Cursor: &base.Cursor{OccurredAt: time.Now(), ID: "0192f0c4-0000-7000-8000-000000000002"},
	}
	sql, args := buildQuery(f, 10)
	if strings.Count(sql, "$") != len(args) {
		t.Fatalf("placeholder %d != args %d: %s", strings.Count(sql, "$"), len(args), sql)
	}
	for _, bad := range []string{"DROP", "x'", f.ActorID, f.ProjectID, "a.b"} {
		if strings.Contains(sql, bad) {
			t.Fatalf("nilai %q muncul di SQL: %s", bad, sql)
		}
	}
	if !strings.Contains(sql, "(occurred_at, id) < (") || !strings.Contains(sql, "ANY(") {
		t.Fatal(sql)
	}
	for _, col := range []string{"args", "result", "user_agent", "trace_id", " ip"} {
		if strings.Contains(sql, col) {
			t.Fatalf("kolom %q tidak boleh dibaca", col)
		}
	}
}

func TestBuildQuery_ILikeEscape(t *testing.T) {
	sql, args := buildQuery(base.Filter{ActorLabel: "50%_"}, 1)
	if !strings.Contains(sql, `actor_label ILIKE $1 ESCAPE '\'`) {
		t.Fatal(sql)
	}
	if args[0] != `%50\%\_%` {
		t.Fatalf("arg = %v", args[0])
	}
}
