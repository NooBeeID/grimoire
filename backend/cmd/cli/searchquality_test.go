package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

var sqNow = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

const sqProject = "0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10"

func sqReport() searchquality.Report {
	counts := []int{38, 12, 5, 2, 2, 1, 0, 1, 0, 0}
	var b []searchquality.RankBucket
	for i, c := range counts {
		b = append(b, searchquality.RankBucket{Label: string(rune('0' + i + 1)), Count: c})
	}
	b[9].Label = "10"
	b = append(b, searchquality.RankBucket{Label: searchquality.NotInResults, Count: 9})
	return searchquality.Report{
		ProjectID:     sqProject,
		Since:         time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		Until:         time.Date(2026, 10, 21, 9, 15, 2, 0, time.UTC),
		WindowSeconds: 120, Searches: 120,
		ZeroResult:     searchquality.Metric{Count: 18, Denominator: 120, Rate: 0.15},
		FollowUp:       searchquality.Metric{Count: 61, Denominator: 100, Rate: 0.61},
		Reformulation:  searchquality.Metric{Count: 27, Denominator: 120, Rate: 0.225},
		RankOpened:     b,
		LegacySearches: 20,
	}
}

type sqSeam struct {
	calls int
	got   searchQualityArgs
	rep   searchquality.Report
	err   error
}

func sqHarness(t *testing.T) (*harness, *sqSeam) {
	t.Helper()
	h := newTestCLI(t, nil, "", false)
	s := &sqSeam{rep: sqReport()}
	h.c.now = func() time.Time { return sqNow }
	h.c.searchQualityRun = func(_ context.Context, a searchQualityArgs) (searchquality.Report, error) {
		s.calls++
		s.got = a
		return s.rep, s.err
	}
	return h, s
}

func TestParseSince(t *testing.T) {
	ok := map[string]string{
		"7d":                        "2026-10-14T10:00:00Z",
		"24h":                       "2026-10-20T10:00:00Z",
		"90m":                       "2026-10-21T08:30:00Z",
		"2026-10-01T08:00:00+07:00": "2026-10-01T01:00:00Z",
		"2026-10-01":                "2026-10-01T00:00:00Z",
	}
	for in, want := range ok {
		got, err := parseSince(in, sqNow)
		if err != nil || got.UTC().Format(time.RFC3339) != want {
			t.Errorf("%q: got %v err %v, want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "0d", "-1d", "-5h", "2026-13-40", "3651d"} {
		if _, err := parseSince(in, sqNow); err == nil {
			t.Errorf("%q harus error", in)
		}
	}
}

func TestSearchQuality_Text(t *testing.T) {
	h, s := sqHarness(t)
	if code := h.run("search-quality", "report", "--project", sqProject, "--since", "7d"); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, h.stderr)
	}
	if s.calls != 1 || !s.got.Since.Equal(sqNow.Add(-7*24*time.Hour)) || !s.got.Until.IsZero() || s.got.Window != 0 || s.got.JSON || s.got.ProjectID != sqProject {
		t.Fatalf("seam: %+v", s.got)
	}
	want := `Kualitas pencarian project 0192f0a4-6c1e-7b3a-9c55-2f1e8d7a4b10
Rentang  2026-10-14T00:00:00Z .. 2026-10-21T09:15:02Z (jendela 120s)
Pencarian                 120
Zero-result               18/120   15.0%
Follow-up                 61/100   61.0%
Reformulasi               27/120   22.5%
Rank dibuka               1:38  2:12  3:5  4:2  5:2  6:1  7:0  8:1  9:0  10:0  tidak di hasil:9
Entri lama dilewati       20 (tanpa result_paths; tidak ikut follow-up/rank)
`
	if h.stdout.String() != want {
		t.Fatalf("golden berbeda:\n%s\nmau:\n%s", h.stdout.String(), want)
	}
}

func TestSearchQuality_ZeroDenominator(t *testing.T) {
	h, s := sqHarness(t)
	s.rep = searchquality.Report{ProjectID: sqProject, WindowSeconds: 120}
	for i := 1; i <= 10; i++ {
		s.rep.RankOpened = append(s.rep.RankOpened, searchquality.RankBucket{Label: itoa(i)})
	}
	s.rep.RankOpened = append(s.rep.RankOpened, searchquality.RankBucket{Label: searchquality.NotInResults})
	if code := h.run("search-quality", "report", "--project", sqProject, "--since", "7d"); code != 0 {
		t.Fatal(code)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "0/0   0.0%") || !strings.Contains(out, "10:0  tidak di hasil:0") {
		t.Fatalf("out=%s", out)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestSearchQuality_JSON(t *testing.T) {
	h, _ := sqHarness(t)
	if code := h.run("search-quality", "report", "--project", sqProject, "--since", "7d", "--json"); code != 0 {
		t.Fatal(code)
	}
	out := h.stdout.Bytes()
	if !json.Valid(out) || strings.Contains(string(out), "Zero-result") {
		t.Fatalf("bukan JSON murni: %s", out)
	}
	var res searchquality.Response
	if err := json.Unmarshal(out, &res); err != nil || res.Report.FollowUp.Count != 61 || len(res.Report.RankOpened) != 11 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr=%s", h.stderr)
	}
}

func TestSearchQuality_FlagsPassed(t *testing.T) {
	h, s := sqHarness(t)
	if code := h.run("search-quality", "report", "--project", sqProject, "--since", "2026-10-14", "--until", "2026-10-21", "--window", "5m"); code != 0 {
		t.Fatal(code)
	}
	if !s.got.Until.Equal(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)) || s.got.Window != 5*time.Minute {
		t.Fatalf("%+v", s.got)
	}
	for _, w := range []string{"1s", "1h"} {
		if code := h.run("search-quality", "report", "--project", sqProject, "--since", "7d", "--window", w); code != 0 {
			t.Errorf("window %s: %d", w, code)
		}
	}
}

func TestSearchQuality_BadUsage(t *testing.T) {
	cases := map[string][]string{
		"tanpa project":   {"report", "--since", "7d"},
		"tanpa since":     {"report", "--project", sqProject},
		"since abc":       {"report", "--project", sqProject, "--since", "abc"},
		"until sebelum":   {"report", "--project", sqProject, "--since", "7d", "--until", "10d"},
		"window 2h":       {"report", "--project", sqProject, "--since", "7d", "--window", "2h"},
		"window 500ms":    {"report", "--project", sqProject, "--since", "7d", "--window", "500ms"},
		"window abc":      {"report", "--project", sqProject, "--since", "7d", "--window", "abc"},
		"argumen sisa":    {"report", "--project", sqProject, "--since", "7d", "foo"},
		"tanpa sub":       {},
		"sub tak dikenal": {"foo"},
	}
	for name, args := range cases {
		h, s := sqHarness(t)
		code := h.run(append([]string{"search-quality"}, args...)...)
		if code != 2 || h.stdout.Len() != 0 || !strings.Contains(h.stderr.String(), "Pemakaian:") || s.calls != 0 {
			t.Errorf("%s: code=%d stdout=%q stderr=%q calls=%d", name, code, h.stdout, h.stderr, s.calls)
		}
	}
	h, _ := sqHarness(t)
	h.run("search-quality", "report", "--since", "7d")
	if !strings.Contains(h.stderr.String(), "--project wajib diisi") {
		t.Errorf("stderr=%s", h.stderr)
	}
}

func TestSearchQuality_Errors(t *testing.T) {
	h, s := sqHarness(t)
	s.err = errs.Wrap(errs.CodeInvalid, "project_id tidak valid", errs.ErrInvalid)
	if code := h.run("search-quality", "report", "--project", "x", "--since", "7d"); code != 1 ||
		!strings.Contains(h.stderr.String(), "search-quality: project_id tidak valid") || h.stdout.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, h.stderr)
	}

	h, s = sqHarness(t)
	s.err = errors.New("secret internal")
	if code := h.run("search-quality", "report", "--project", sqProject, "--since", "7d"); code != 1 ||
		!strings.Contains(h.stderr.String(), "search-quality report gagal; periksa log") ||
		strings.Contains(h.stderr.String(), "secret internal") {
		t.Fatalf("code=%d stderr=%q", code, h.stderr)
	}

	h, s = sqHarness(t)
	s.err = errAlreadyReported
	if code := h.run("search-quality", "report", "--project", sqProject, "--since", "7d"); code != 1 || h.stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, h.stderr)
	}
}

func TestSearchQuality_UsageMentions(t *testing.T) {
	h, _ := sqHarness(t)
	h.run("help")
	if !strings.Contains(h.stdout.String(), "search-quality report") {
		t.Fatal("usage tidak memuat search-quality report")
	}
}
