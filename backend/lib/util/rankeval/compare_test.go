package rankeval

import "testing"

func mk(r1, r5, mrr float64) Metrics { return Metrics{Pairs: 10, Recall1: r1, Recall5: r5, MRR: mrr} }

func TestCompare(t *testing.T) {
	base := mk(0.5, 0.70, 0.6)
	cases := []struct {
		name string
		cur  Metrics
		tol  float64
		want []string
	}{
		{"turun 0.03", mk(0.5, 0.67, 0.6), 0.02, []string{"recall_at_5"}},
		{"naik", mk(0.6, 0.73, 0.7), 0.02, nil},
		{"turun 0.01", mk(0.5, 0.69, 0.6), 0.02, nil},
		{"turun tepat tol", mk(0.5, 0.68, 0.6), 0.02, nil},
		{"semua turun", mk(0.4, 0.6, 0.5), 0.02, []string{"recall_at_1", "recall_at_5", "mrr"}},
		{"campuran", mk(0.6, 0.70, 0.55), 0.02, []string{"mrr"}},
		{"tol negatif", mk(0.5, 0.6999, 0.6), -1, []string{"recall_at_5"}},
		{"tol nol sama", base, 0, nil},
	}
	for _, c := range cases {
		d := Compare(c.cur, base, c.tol)
		if len(d) != len(c.want) {
			t.Errorf("%s: %+v", c.name, d)
			continue
		}
		for i, w := range c.want {
			if d[i].Metric != w || d[i].Delta <= 0 {
				t.Errorf("%s: %+v", c.name, d[i])
			}
		}
	}
}

func TestCompareDelta(t *testing.T) {
	d := Compare(mk(0.5, 0.67, 0.6), mk(0.5, 0.70, 0.6), 0.02)
	if len(d) != 1 || !near(d[0].Delta, 0.03) || d[0].Base != 0.7 || d[0].Cur != 0.67 {
		t.Errorf("%+v", d)
	}
}

func TestCompareIgnoresPerKind(t *testing.T) {
	base, cur := mk(0.5, 0.5, 0.5), mk(0.5, 0.5, 0.5)
	base.PerKind = map[string]KindMetrics{KindTypo: {Pairs: 1, Recall5: 1}}
	cur.PerKind = map[string]KindMetrics{KindTypo: {Pairs: 1, Recall5: 0}}
	if d := Compare(cur, base, 0.02); len(d) != 0 {
		t.Errorf("%+v", d)
	}
}
