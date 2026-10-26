package base

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

func TestSummaryQuery_Validate(t *testing.T) {
	q := SummaryQuery{}
	if err := q.Validate(); err != nil || q.Range != Range7d {
		t.Fatalf("range=%q err=%v", q.Range, err)
	}
	for _, r := range []Range{Range24h, Range7d, Range30d} {
		q := SummaryQuery{Range: r}
		if err := q.Validate(); err != nil {
			t.Fatalf("%s: %v", r, err)
		}
	}
	for _, r := range []Range{"1d", "7D", "30d ", "90d"} {
		q := SummaryQuery{Range: r}
		if f := fieldOf(t, q.Validate()); f != "range" {
			t.Fatalf("%q field=%s", r, f)
		}
	}
	q = SummaryQuery{ProjectID: "not-a-uuid"}
	if f := fieldOf(t, q.Validate()); f != "project_id" {
		t.Fatalf("field=%s", f)
	}
	q = SummaryQuery{ProjectID: "7F9C2C1E-0000-4000-8000-000000000001"}
	if err := q.Validate(); err != nil || q.ProjectID != strings.ToLower(q.ProjectID) {
		t.Fatalf("pid=%q err=%v", q.ProjectID, err)
	}
	q = SummaryQuery{Range: "x"}
	if !errors.Is(q.Validate(), errs.ErrInvalid) {
		t.Fatal("want ErrInvalid")
	}
}

func TestNewWindow(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 30, 15, 999, time.FixedZone("x", 7*3600))
	cases := map[Range]struct {
		d time.Duration
		b Bucket
	}{
		Range24h: {24 * time.Hour, BucketHour},
		Range7d:  {168 * time.Hour, BucketDay},
		Range30d: {720 * time.Hour, BucketDay},
	}
	for r, c := range cases {
		w := NewWindow(r, now)
		if w.To.Sub(w.From) != c.d || w.Bucket != c.b || w.To.Nanosecond() != 0 ||
			w.To.Location() != time.UTC || w.From.Location() != time.UTC {
			t.Fatalf("%s: %+v", r, w)
		}
	}
}

func TestBucketStarts(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	w := NewWindow(Range24h, at("2026-10-22T10:30:15Z"))
	bs := w.BucketStarts()
	if len(bs) != 25 || !bs[0].Equal(at("2026-10-21T10:00:00Z")) {
		t.Fatalf("24h: %d %v", len(bs), bs[0])
	}
	if n := len(NewWindow(Range24h, at("2026-10-22T10:00:00Z")).BucketStarts()); n != 24 {
		t.Fatalf("24h aligned: %d", n)
	}
	for r, want := range map[Range][2]int{Range7d: {7, 8}, Range30d: {30, 31}} {
		w := NewWindow(r, at("2026-10-22T10:30:15Z"))
		if n := len(w.BucketStarts()); n != want[1] {
			t.Fatalf("%s: %d", r, n)
		}
		if n := len(NewWindow(r, at("2026-10-22T00:00:00Z")).BucketStarts()); n != want[0] {
			t.Fatalf("%s aligned: %d", r, n)
		}
	}
	for _, r := range []Range{Range24h, Range7d, Range30d} {
		w := NewWindow(r, at("2026-10-22T10:30:15Z"))
		for _, b := range w.BucketStarts() {
			if !b.Equal(b.Truncate(w.Bucket.Step())) || !b.Before(w.To) {
				t.Fatalf("%s bucket %v", r, b)
			}
		}
	}
}

func TestToolActions_Completeness(t *testing.T) {
	tools, gw := map[string]bool{}, map[string]bool{}
	for _, a := range ToolActions() {
		tools[a] = true
		if !strings.HasPrefix(a, "mcp.") {
			t.Fatalf("non-mcp: %s", a)
		}
	}
	for _, a := range GatewayActions() {
		gw[a] = true
	}
	for _, a := range record.Actions() {
		if !strings.HasPrefix(a, "mcp.") {
			continue
		}
		if tools[a] == gw[a] {
			t.Fatalf("%s harus tepat satu daftar (tool=%v gw=%v)", a, tools[a], gw[a])
		}
	}
	want := []string{"mcp.initialize", "mcp.tools_list", "mcp.auth.rejected", "mcp.rate_limited", "mcp.unknown_tool"}
	if len(GatewayActions()) != 5 {
		t.Fatalf("gateway=%v", GatewayActions())
	}
	for _, a := range want {
		if !gw[a] {
			t.Fatalf("missing %s", a)
		}
	}
	if len(ToolActions()) != 9 || !slices.Contains(ToolActions(), "mcp.read_section") {
		t.Fatalf("tools=%v", ToolActions())
	}
	if ToolName("mcp.read_file") != "read_file" {
		t.Fatal("ToolName")
	}
}

func f64(v float64) *float64 { return &v }

func TestBuildReport(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	w := NewWindow(Range7d, now)
	var traffic []TrafficPoint
	for _, s := range w.BucketStarts() {
		traffic = append(traffic, TrafficPoint{At: s})
	}
	agg := Aggregates{
		Tools: []ToolAgg{
			{Action: "mcp.read_file", Calls: 5, OK: 5, P50Ms: f64(1.2345), P95Ms: f64(9.999)},
			{Action: "mcp.list_files", Calls: 10, OK: 9, Errors: 1},
			{Action: "mcp.get_api_endpoint", Calls: 5, OK: 5},
		},
		Projects: []ProjectAgg{{ProjectID: "p1", ProjectLabel: ""}, {ProjectID: "p2", ProjectLabel: "Two", Calls: 3}},
		Traffic:  traffic,
	}
	r, err := BuildReport(w, SummaryQuery{Range: Range7d}, agg, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{r.ToolMetrics[0].Tool, r.ToolMetrics[1].Tool, r.ToolMetrics[2].Tool}
	if strings.Join(order, ",") != "list_files,get_api_endpoint,read_file" {
		t.Fatalf("order=%v", order)
	}
	if r.ToolMetrics[0].ErrorRate != 0.1 || r.ToolUsage[0].Tool != "list_files" {
		t.Fatalf("%+v", r.ToolMetrics[0])
	}
	if *r.ToolMetrics[2].P50Ms != 1.23 || *r.ToolMetrics[2].P95Ms != 10 {
		t.Fatalf("round %v %v", *r.ToolMetrics[2].P50Ms, *r.ToolMetrics[2].P95Ms)
	}
	if r.TotalCalls != 20 || r.TotalErrors != 1 || *r.ErrorRate != 0.05 {
		t.Fatalf("totals %+v", r)
	}
	if r.TopProjects[0].ProjectLabel != "p1" || r.TopProjects[1].ProjectLabel != "Two" || r.ProjectID != nil {
		t.Fatalf("top %+v", r.TopProjects)
	}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"p50_ms":null`) || !strings.Contains(string(b), `"search_quality":null`) {
		t.Fatalf("json %s", b)
	}

	empty, err := BuildReport(w, SummaryQuery{Range: Range7d, ProjectID: "abc"}, Aggregates{Traffic: traffic}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(empty)
	s := string(b)
	for _, k := range []string{`"error_rate":null`, `"top_projects":[]`, `"tool_usage":[]`, `"tool_metrics":[]`, `"project_id":"abc"`} {
		if !strings.Contains(s, k) {
			t.Fatalf("missing %s in %s", k, s)
		}
	}

	if _, err := BuildReport(w, SummaryQuery{}, Aggregates{Traffic: traffic[1:]}, nil); err == nil {
		t.Fatal("want length error")
	}
	shifted := append([]TrafficPoint(nil), traffic...)
	shifted[0].At = shifted[0].At.Add(time.Hour)
	if _, err := BuildReport(w, SummaryQuery{}, Aggregates{Traffic: shifted}, nil); err == nil {
		t.Fatal("want start error")
	}
}

func TestQualityFromReport(t *testing.T) {
	var ranks []searchquality.RankBucket
	for i := 0; i < 11; i++ {
		ranks = append(ranks, searchquality.RankBucket{Label: "x", Count: i})
	}
	rep := searchquality.Report{
		WindowSeconds: 120, Searches: 7, LegacySearches: 2,
		ZeroResult:    searchquality.Metric{Count: 1, Denominator: 4, Rate: 0.25},
		FollowUp:      searchquality.Metric{Count: 2, Denominator: 5, Rate: 0.4},
		Reformulation: searchquality.Metric{Count: 3, Denominator: 7, Rate: 3.0 / 7},
		RankOpened:    ranks,
	}
	q := QualityFromReport(rep)
	if !q.Available || *q.Searches != 7 || *q.WindowSeconds != 120 || *q.LegacySearches != 2 ||
		q.ZeroResult.Count != 1 || q.FollowUp.Denominator != 5 || q.Reformulation.Count != 3 || len(q.RankOpened) != 11 {
		t.Fatalf("%+v", q)
	}
	b, _ := json.Marshal(QualityUnavailable())
	if string(b) != `{"available":false,"error_code":"UNAVAILABLE"}` {
		t.Fatalf("%s", b)
	}
}
