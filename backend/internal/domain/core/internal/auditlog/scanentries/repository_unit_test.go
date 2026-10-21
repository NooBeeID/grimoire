package scanentries

import (
	"strings"
	"testing"

	"grimoire/lib/contracts/core/audit/scanentries"
)

func baseParams() params {
	return params{ProjectID: pid, Actions: []string{"a.b"}, From: t0, To: now, Limit: 3}
}

func TestBuildQuery_NoAfter(t *testing.T) {
	q, args := buildQuery(baseParams(), 4)
	for _, s := range []string{"project_id = $1::uuid", "ORDER BY occurred_at ASC, id ASC LIMIT $5"} {
		if !strings.Contains(q, s) {
			t.Fatalf("missing %q in %s", s, q)
		}
	}
	if strings.Contains(q, "(occurred_at, id) >") || len(args) != 5 {
		t.Fatalf("q=%s args=%d", q, len(args))
	}
}

func TestBuildQuery_After(t *testing.T) {
	p := baseParams()
	p.After = &scanentries.Cursor{OccurredAt: t0, ID: pid}
	q, args := buildQuery(p, 4)
	if !strings.Contains(q, "(occurred_at, id) > ($5::timestamptz, $6::uuid)") || !strings.Contains(q, "LIMIT $7") {
		t.Fatal(q)
	}
	if strings.Count(q, "$") != len(args) || len(args) != 7 {
		t.Fatalf("args=%d q=%s", len(args), q)
	}
}

func TestBuildQuery_NoInjectionAndMinimal(t *testing.T) {
	p := baseParams()
	p.Actions = []string{"x'; DROP TABLE audit_log; --"}
	q, _ := buildQuery(p, 1)
	if strings.Contains(q, "DROP") {
		t.Fatal(q)
	}
	up := strings.ToUpper(q)
	for _, bad := range []string{"UPDATE", "DELETE", "TRUNCATE"} {
		if strings.Contains(up, bad) {
			t.Fatalf("contains %s", bad)
		}
	}
	if strings.Count(up, "SELECT") != 1 {
		t.Fatal(q)
	}
	for _, col := range []string{"actor_label", "ip", "user_agent", "trace_id", "project_label"} {
		if strings.Contains(q, col) {
			t.Fatalf("selects %s", col)
		}
	}
}

func TestDecodeObject(t *testing.T) {
	for _, in := range [][]byte{nil, []byte(""), []byte("null")} {
		m, err := decodeObject(in)
		if err != nil || m == nil || len(m) != 0 {
			t.Fatalf("in=%q m=%v err=%v", in, m, err)
		}
	}
	m, err := decodeObject([]byte(`{"a":1}`))
	if err != nil || m["a"] != float64(1) {
		t.Fatalf("m=%v err=%v", m, err)
	}
	if _, err := decodeObject([]byte(`[1]`)); err == nil {
		t.Fatal("want error")
	}
}
