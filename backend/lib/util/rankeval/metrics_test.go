package rankeval

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRecallAtK(t *testing.T) {
	cases := []struct {
		name   string
		ranked []string
		expect []string
		k      int
		want   float64
	}{
		{"rank1 k1", []string{"a", "b"}, []string{"a"}, 1, 1},
		{"rank2 k1", []string{"b", "a"}, []string{"a"}, 1, 0},
		{"rank2 k5", []string{"b", "a"}, []string{"a"}, 5, 1},
		{"dua expect k5", []string{"x", "a", "y", "z", "w", "v", "b"}, []string{"a", "b"}, 5, 0.5},
		{"dua expect k10", []string{"x", "a", "y", "z", "w", "v", "b"}, []string{"a", "b"}, 10, 1},
		{"k nol", []string{"a"}, []string{"a"}, 0, 0},
		{"k negatif", []string{"a"}, []string{"a"}, -1, 0},
		{"expect kosong", []string{"a"}, nil, 5, 0},
		{"ranked nil", nil, []string{"a"}, 5, 0},
		{"duplikat ranked", []string{"a", "a", "a"}, []string{"a", "b"}, 5, 0.5},
	}
	for _, c := range cases {
		got := RecallAtK(c.ranked, c.expect, c.k)
		if math.IsNaN(got) || !near(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestReciprocalRank(t *testing.T) {
	eleven := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "a"}
	cases := []struct {
		name   string
		ranked []string
		expect []string
		want   float64
	}{
		{"rank1", []string{"a"}, []string{"a"}, 1},
		{"rank3", []string{"x", "y", "a"}, []string{"a"}, 1.0 / 3},
		{"tidak ada", []string{"x"}, []string{"a"}, 0},
		{"posisi 11", eleven, []string{"a"}, 0},
		{"terdekat", []string{"x", "b", "a"}, []string{"a", "b"}, 0.5},
		{"nil", nil, []string{"a"}, 0},
	}
	for _, c := range cases {
		if got := ReciprocalRank(c.ranked, c.expect); !near(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func testGolden() Golden {
	return Golden{Pairs: []Pair{
		{Query: "q1", Expect: []string{"a"}, Kind: KindKeyword},
		{Query: "q2", Expect: []string{"b", "c"}, Kind: KindKeyword},
		{Query: "q3", Expect: []string{"d"}, Kind: KindTypo},
	}}
}

func TestEvaluate(t *testing.T) {
	g := testGolden()
	res := map[string][]string{
		"q1":    {"a"},                          // r1=1 r5=1 rr=1
		"q2":    {"x", "b", "y", "z", "w", "c"}, // r1=0 r5=0.5 rr=0.5
		"extra": {"zzz"},                        // diabaikan; q3 tak ada -> 0
	}
	m := g.Evaluate(res)
	if m.Pairs != 3 {
		t.Fatalf("pairs=%d", m.Pairs)
	}
	if !near(m.Recall1, 1.0/3) || !near(m.Recall5, 1.5/3) || !near(m.MRR, 1.5/3) {
		t.Errorf("metrics %+v", m)
	}
	kw := m.PerKind[KindKeyword]
	if kw.Pairs != 2 || !near(kw.Recall1, 0.5) || !near(kw.Recall5, 0.75) || !near(kw.MRR, 0.75) {
		t.Errorf("keyword %+v", kw)
	}
	if ty := m.PerKind[KindTypo]; ty.Pairs != 1 || ty.MRR != 0 {
		t.Errorf("typo %+v", ty)
	}
	if _, ok := m.PerKind[KindPhrase]; ok {
		t.Error("kind tanpa pasangan tidak boleh muncul")
	}
}

func TestEvaluateEmpty(t *testing.T) {
	m := Golden{}.Evaluate(nil)
	if m.Pairs != 0 || math.IsNaN(m.MRR) {
		t.Errorf("%+v", m)
	}
}

func TestFailures(t *testing.T) {
	g := testGolden()
	res := map[string][]string{
		"q1": {"a"},
		"q2": {"x", "b", "y", "z", "w", "c", "v"},
	}
	f := g.Failures(res)
	if len(f) != 2 || f[0].Pair.Query != "q2" || f[1].Pair.Query != "q3" {
		t.Fatalf("failures %+v", f)
	}
	if f[0].Rank != 2 || len(f[0].Got) != 5 {
		t.Errorf("q2 %+v", f[0])
	}
	if f[1].Rank != 0 || len(f[1].Got) != 0 {
		t.Errorf("q3 %+v", f[1])
	}
}
