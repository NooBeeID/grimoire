package base

import (
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
)

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("want errs.ErrInvalid, got %v", err)
	}
	return e.Details.(map[string]string)["field"]
}

func TestFilter_Defaults(t *testing.T) {
	f := Filter{}
	if err := f.Validate(); err != nil || f.Limit != DefaultLimit {
		t.Fatalf("limit=%d err=%v", f.Limit, err)
	}
	f = Filter{Statuses: []string{"ok", "ok", "denied"}}
	if err := f.Validate(); err != nil || len(f.Statuses) != 2 {
		t.Fatalf("statuses=%v err=%v", f.Statuses, err)
	}
}

func TestFilter_Invalid(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	acts := make([]string, 21)
	for i := range acts {
		acts[i] = "mcp.read_file"
	}
	cases := []struct {
		name  string
		f     Filter
		field string
	}{
		{"limit 201", Filter{Limit: 201}, "limit"},
		{"limit -1", Filter{Limit: -1}, "limit"},
		{"21 actions", Filter{Actions: acts}, "action"},
		{"bad action", Filter{Actions: []string{"BAD"}}, "action"},
		{"bad status", Filter{Statuses: []string{"x"}}, "status"},
		{"from>=to", Filter{From: t0, To: t0}, "from"},
		{"bad actor id", Filter{ActorID: "abc"}, "actor_id"},
		{"bad project id", Filter{ProjectID: "abc"}, "project_id"},
		{"bad actor type", Filter{ActorType: "admin"}, "actor_type"},
		{"long text", Filter{ActorLabel: strings.Repeat("a", 201)}, "actor_label"},
		{"long target", Filter{Target: strings.Repeat("a", 201)}, "target"},
	}
	for _, c := range cases {
		f := c.f
		if got := fieldOf(t, f.Validate()); got != c.field {
			t.Errorf("%s: field=%q want %q", c.name, got, c.field)
		}
	}
}

func TestFilter_Valid(t *testing.T) {
	f := Filter{From: time.Now(), ActorID: uid1, ProjectID: uid2, Actions: []string{"mcp.read_file"}, Limit: 200}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := EscapeLike(`50%_a\`); got != `50\%\_a\\` {
		t.Fatalf("got %q", got)
	}
}
