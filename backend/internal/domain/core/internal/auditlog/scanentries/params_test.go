package scanentries

import (
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/scanentries"
)

const (
	pid      = "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10"
	sentinel = "SECRET-XYZ"
)

var (
	t0  = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	now = time.Date(2026, 10, 21, 0, 2, 0, 0, time.UTC)
)

func validReq() scanentries.Request {
	return scanentries.Request{
		ProjectID: pid,
		Actions:   []string{"mcp.search_knowledge", "mcp.read_file"},
		From:      t0,
	}
}

func TestParse_ValidDefaults(t *testing.T) {
	p, err := parse(validReq(), now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Limit != scanentries.DefaultLimit || !p.To.Equal(now) || p.To.Location() != time.UTC {
		t.Fatalf("p=%+v", p)
	}
}

func TestParse_NonUTCNormalised(t *testing.T) {
	in := validReq()
	loc := time.FixedZone("x", 7*3600)
	in.From = t0.In(loc)
	in.To = t0.Add(time.Hour).In(loc)
	in.After = &scanentries.Cursor{OccurredAt: t0.In(loc), ID: pid}
	p, err := parse(in, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.From.Location() != time.UTC || p.To.Location() != time.UTC || p.After.OccurredAt.Location() != time.UTC {
		t.Fatalf("p=%+v", p)
	}
}

func TestParse_Invalid(t *testing.T) {
	long := strings.Repeat("a", 129) + ".b"
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = "a.b" + strings.Repeat("c", i)
	}
	cases := []struct {
		name, field string
		mut         func(*scanentries.Request)
	}{
		{"pid empty", "project_id", func(r *scanentries.Request) { r.ProjectID = "" }},
		{"pid junk", "project_id", func(r *scanentries.Request) { r.ProjectID = sentinel }},
		{"pid upper", "project_id", func(r *scanentries.Request) { r.ProjectID = strings.ToUpper(pid) }},
		{"pid braces", "project_id", func(r *scanentries.Request) { r.ProjectID = "{" + pid + "}" }},
		{"pid urn", "project_id", func(r *scanentries.Request) { r.ProjectID = "urn:uuid:" + pid }},
		{"pid nodash", "project_id", func(r *scanentries.Request) { r.ProjectID = strings.ReplaceAll(pid, "-", "") }},
		{"actions nil", "actions", func(r *scanentries.Request) { r.Actions = nil }},
		{"actions empty", "actions", func(r *scanentries.Request) { r.Actions = []string{} }},
		{"actions 11", "actions", func(r *scanentries.Request) { r.Actions = eleven }},
		{"action blank", "actions", func(r *scanentries.Request) { r.Actions = []string{""} }},
		{"action upper", "actions", func(r *scanentries.Request) { r.Actions = []string{"Bad.Action"} }},
		{"action nodot", "actions", func(r *scanentries.Request) { r.Actions = []string{"noDot"} }},
		{"action dots", "actions", func(r *scanentries.Request) { r.Actions = []string{"a..b"} }},
		{"action sentinel", "actions", func(r *scanentries.Request) { r.Actions = []string{sentinel} }},
		{"action long", "actions", func(r *scanentries.Request) { r.Actions = []string{long} }},
		{"from zero", "from", func(r *scanentries.Request) { r.From = time.Time{} }},
		{"to equal", "to", func(r *scanentries.Request) { r.To = t0 }},
		{"to before", "to", func(r *scanentries.Request) { r.To = t0.Add(-time.Second) }},
		{"limit neg", "limit", func(r *scanentries.Request) { r.Limit = -1 }},
		{"limit big", "limit", func(r *scanentries.Request) { r.Limit = 1001 }},
		{"after zero time", "after", func(r *scanentries.Request) { r.After = &scanentries.Cursor{ID: pid} }},
		{"after empty id", "after", func(r *scanentries.Request) { r.After = &scanentries.Cursor{OccurredAt: t0} }},
		{"after bad id", "after", func(r *scanentries.Request) { r.After = &scanentries.Cursor{OccurredAt: t0, ID: sentinel} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := validReq()
			c.mut(&in)
			_, err := parse(in, now)
			if !errors.Is(err, errs.ErrInvalid) {
				t.Fatalf("err=%v", err)
			}
			var e *errs.Error
			if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
				t.Fatalf("err=%v", err)
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("leaks input: %v", err)
			}
			d, _ := e.Details.(map[string]string)
			if d["field"] != c.field {
				t.Fatalf("details=%#v want field %s", e.Details, c.field)
			}
		})
	}
}

func TestParse_Boundaries(t *testing.T) {
	ten := make([]string, 10)
	for i := range ten {
		ten[i] = "a.b" + strings.Repeat("c", i)
	}
	ok := []func(*scanentries.Request){
		func(r *scanentries.Request) { r.Actions = ten },
		func(r *scanentries.Request) { r.To = t0.Add(time.Nanosecond) },
		func(r *scanentries.Request) { r.Limit = 1 },
		func(r *scanentries.Request) { r.Limit = 1000 },
		func(r *scanentries.Request) { r.Actions = []string{"a.b", "a.b", "a.b"} },
	}
	for i, m := range ok {
		in := validReq()
		m(&in)
		if _, err := parse(in, now); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestParse_DedupKeepsFirstOrder(t *testing.T) {
	in := validReq()
	in.Actions = []string{"b.x", "a.x", "b.x"}
	p, err := parse(in, now)
	if err != nil || len(p.Actions) != 2 || p.Actions[0] != "b.x" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}
