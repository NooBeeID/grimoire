package base

import (
	"encoding/json"
	"testing"
	"time"

	"grimoire/lib/contracts/core/audit/record"
)

var qt0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

var (
	qSince = qt0.Add(-time.Hour)
	qUntil = qt0.Add(time.Hour)
)

type seq struct {
	n  int
	ev []AuditEvent
}

func (s *seq) search(actor, q string, at time.Time, total int, paths []string) {
	s.n++
	e := AuditEvent{ID: idOf(s.n), ActorID: actor, Action: record.ActionMCPSearchKnowledge, Status: record.StatusOK,
		At: at, Query: q, Total: total, HasTotal: true}
	if paths != nil {
		e.Paths, e.HasPaths = paths, true
	}
	s.ev = append(s.ev, e)
}

func (s *seq) read(actor, target string, at time.Time) {
	s.n++
	s.ev = append(s.ev, AuditEvent{ID: idOf(s.n), ActorID: actor, Action: record.ActionMCPReadFile, Status: record.StatusOK, At: at, Target: target})
}

func idOf(n int) string { return string(rune('a'+n/26)) + string(rune('a'+n%26)) }

func (s *seq) report() QualityReport {
	a := NewQualityAccumulator(qSince, qUntil, DefaultQualityWindow)
	for _, e := range s.ev {
		a.Add(e)
	}
	return a.Report("p")
}

func bucket(r QualityReport, label string) int {
	for _, b := range r.RankOpened {
		if b.Label == label {
			return b.Count
		}
	}
	return -1
}

var p3 = []string{"p1", "p2", "p3"}

func TestEventFromAudit(t *testing.T) {
	for _, tot := range []any{float64(3), json.Number("3"), int(3)} {
		e := EventFromAudit("i", "mcp.search_knowledge", "a", "", "ok", qt0, map[string]any{"query": "q"}, map[string]any{"total": tot})
		if !e.HasTotal || e.Total != 3 || e.Query != "q" {
			t.Fatalf("%T %+v", tot, e)
		}
	}
	if e := EventFromAudit("i", "x", "a", "", "ok", qt0, nil, map[string]any{"total": "3"}); e.HasTotal {
		t.Fatal("string total")
	}
	e := EventFromAudit("i", "x", "a", "", "ok", qt0, nil, map[string]any{"result_paths": []any{"a", "b"}})
	if !e.HasPaths || len(e.Paths) != 2 {
		t.Fatalf("%+v", e)
	}
	if e := EventFromAudit("i", "x", "a", "", "ok", qt0, nil, map[string]any{"result_paths": []any{"a", 1}}); e.HasPaths {
		t.Fatal("non-string")
	}
	if e := EventFromAudit("i", "x", "a", "", "ok", qt0, nil, map[string]any{}); e.HasPaths {
		t.Fatal("missing")
	}
	if e := EventFromAudit("i", "x", "a", "", "ok", qt0, nil, nil); e.HasPaths || e.HasTotal {
		t.Fatal("nil maps")
	}
	if e := EventFromAudit("i", "x", "a", "", "ok", qt0, nil, map[string]any{"result_paths": []any{}}); !e.HasPaths {
		t.Fatal("empty array is paths")
	}
}

func TestNormalizeQuery(t *testing.T) {
	if NormalizeQuery("  Login  OAuth ") != NormalizeQuery("login oauth") {
		t.Fatal()
	}
}

func TestQuality_FollowUpRank(t *testing.T) {
	var s seq
	s.search("a", "x", qt0, 3, p3)
	s.read("a", "p2", qt0.Add(30*time.Second))
	r := s.report()
	if r.FollowUp.Count != 1 || r.FollowUp.Denominator != 1 || r.FollowUp.Rate != 1 || bucket(r, "2") != 1 {
		t.Fatalf("%+v", r)
	}
	if len(r.RankOpened) != 11 || r.RankOpened[10].Label != "not_in_results" || r.RankOpened[0].Label != "1" {
		t.Fatalf("%+v", r.RankOpened)
	}
}

func TestQuality_FollowUpWindow(t *testing.T) {
	cases := []struct {
		name  string
		actor string
		at    time.Duration
		want  int
	}{
		{"actor lain", "b", 30 * time.Second, 0},
		{"lewat jendela", "a", 180 * time.Second, 0},
		{"tepat batas", "a", 120 * time.Second, 1},
		{"delta nol", "a", 0, 0},
	}
	for _, c := range cases {
		var s seq
		s.search("a", "x", qt0, 3, p3)
		s.read(c.actor, "p1", qt0.Add(c.at))
		if got := s.report().FollowUp.Count; got != c.want {
			t.Errorf("%s: %d want %d", c.name, got, c.want)
		}
	}
}

func TestQuality_NotInResults(t *testing.T) {
	var s seq
	s.search("a", "x", qt0, 3, p3)
	s.read("a", "p9", qt0.Add(10*time.Second))
	r := s.report()
	if bucket(r, "not_in_results") != 1 || r.FollowUp.Count != 0 || r.FollowUp.Denominator != 1 {
		t.Fatalf("%+v", r)
	}
	s = seq{}
	s.search("a", "x", qt0, 3, p3)
	s.read("a", "p9", qt0.Add(10*time.Second))
	s.read("a", "p1", qt0.Add(20*time.Second))
	r = s.report()
	if bucket(r, "1") != 1 || bucket(r, "not_in_results") != 0 || r.FollowUp.Count != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestQuality_Reformulation(t *testing.T) {
	var s seq
	s.search("a", "login", qt0, 1, p3)
	s.search("a", "oauth login", qt0.Add(60*time.Second), 1, p3)
	r := s.report()
	if r.Reformulation.Count != 1 || r.Reformulation.Denominator != 2 {
		t.Fatalf("%+v", r.Reformulation)
	}
	for name, q2 := range map[string]string{"sama": "login", "kapital": " LOGIN "} {
		s = seq{}
		s.search("a", "login", qt0, 1, p3)
		s.search("a", q2, qt0.Add(10*time.Second), 1, p3)
		if c := s.report().Reformulation.Count; c != 0 {
			t.Errorf("%s: %d", name, c)
		}
	}
	s = seq{}
	s.search("a", "login", qt0, 1, p3)
	s.search("a", "oauth", qt0.Add(130*time.Second), 1, p3)
	if c := s.report().Reformulation.Count; c != 0 {
		t.Errorf("lewat jendela: %d", c)
	}
	s = seq{}
	s.search("a", "login", qt0, 1, p3)
	s.search("a", "oauth", qt0, 1, p3)
	if c := s.report().Reformulation.Count; c != 0 {
		t.Errorf("delta nol: %d", c)
	}
}

func TestQuality_ReformulationOnlyNext(t *testing.T) {
	var s seq
	s.search("a", "x", qt0, 1, p3)
	s.search("a", "X", qt0.Add(10*time.Second), 1, p3)
	s.search("a", "y", qt0.Add(20*time.Second), 1, p3)
	r := s.report()
	if r.Reformulation.Count != 1 || r.Reformulation.Denominator != 3 {
		t.Fatalf("%+v", r.Reformulation)
	}
}

func TestQuality_Legacy(t *testing.T) {
	var s seq
	s.search("a", "x", qt0, 0, nil)
	r := s.report()
	if r.ZeroResult.Count != 1 || r.ZeroResult.Denominator != 1 || r.LegacySearches != 1 || r.FollowUp.Denominator != 0 || r.FollowUp.Rate != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestQuality_IgnoresErrorsAndAnonymous(t *testing.T) {
	var s seq
	s.search("a", "x", qt0, 3, p3)
	s.ev[0].Status = record.StatusError
	s.search("a", "y", qt0.Add(5*time.Second), 3, p3)
	s.read("a", "p1", qt0.Add(10*time.Second))
	s.ev[2].Status = record.StatusError
	s.search("", "z", qt0.Add(15*time.Second), 3, p3)
	r := s.report()
	if r.Searches != 1 || r.FollowUp.Count != 0 || r.Reformulation.Count != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestQuality_RangeBoundaries(t *testing.T) {
	var s seq
	s.search("a", "x", qUntil.Add(-10*time.Second), 3, p3)
	s.search("a", "y", qUntil, 3, p3) // di luar rentang; tetap memicu reformulasi
	s.read("a", "p1", qUntil.Add(30*time.Second))
	r := s.report()
	if r.Searches != 1 || r.Reformulation.Count != 1 || r.FollowUp.Count != 1 || r.FollowUp.Denominator != 1 {
		t.Fatalf("%+v", r)
	}
	// sebelum qSince diabaikan
	s = seq{}
	s.search("a", "x", qSince.Add(-time.Second), 3, p3)
	if s.report().Searches != 0 {
		t.Fatal("sebelum qSince")
	}
}

func TestQuality_Empty(t *testing.T) {
	r := NewQualityAccumulator(qSince, qUntil, 0).Report("p")
	if len(r.RankOpened) != 11 || r.WindowSeconds != 120 || r.Searches != 0 || r.ZeroResult.Rate != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestQuality_Eviction(t *testing.T) {
	a := NewQualityAccumulator(qt0, qt0.Add(100000*time.Hour), DefaultQualityWindow)
	for i := 0; i < 10000; i++ {
		a.Add(AuditEvent{ID: idOf(i), ActorID: "a", Action: record.ActionMCPSearchKnowledge, Status: record.StatusOK,
			At: qt0.Add(time.Duration(i) * 200 * time.Second), Query: "q", HasTotal: true, Total: 1, HasPaths: true, Paths: p3})
		if n := len(a.open["a"]); n > 1 {
			t.Fatalf("antrean %d", n)
		}
	}
}
