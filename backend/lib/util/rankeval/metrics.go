package rankeval

// RecallAtK = |expect ∩ ranked[:k]| / |expect|.
func RecallAtK(ranked, expect []string, k int) float64 {
	if k <= 0 || len(expect) == 0 {
		return 0
	}
	if k > len(ranked) {
		k = len(ranked)
	}
	top := make(map[string]bool, k)
	for _, r := range ranked[:k] {
		top[r] = true
	}
	want := make(map[string]bool, len(expect))
	for _, e := range expect {
		want[e] = true
	}
	hit := 0
	for e := range want {
		if top[e] {
			hit++
		}
	}
	return float64(hit) / float64(len(want))
}

// firstRank mengembalikan rank 1-based expect pertama di ranked[:TopK]; 0 bila tidak ada.
func firstRank(ranked, expect []string) int {
	want := make(map[string]bool, len(expect))
	for _, e := range expect {
		want[e] = true
	}
	for i, r := range ranked {
		if i >= TopK {
			break
		}
		if want[r] {
			return i + 1
		}
	}
	return 0
}

// ReciprocalRank = 1/rank expect pertama dalam ranked[:TopK]; 0 bila tidak ada.
func ReciprocalRank(ranked, expect []string) float64 {
	r := firstRank(ranked, expect)
	if r == 0 {
		return 0
	}
	return 1 / float64(r)
}

type KindMetrics struct {
	Pairs   int     `json:"pairs"`
	Recall1 float64 `json:"recall_at_1"`
	Recall5 float64 `json:"recall_at_5"`
	MRR     float64 `json:"mrr"`
}

type Metrics struct {
	Pairs   int                    `json:"pairs"`
	Recall1 float64                `json:"recall_at_1"`
	Recall5 float64                `json:"recall_at_5"`
	MRR     float64                `json:"mrr"`
	PerKind map[string]KindMetrics `json:"per_kind"`
}

// Evaluate menghitung metrik; results = map[query][]path berurut peringkat.
func (g Golden) Evaluate(results map[string][]string) Metrics {
	m := Metrics{Pairs: len(g.Pairs), PerKind: map[string]KindMetrics{}}
	for _, p := range g.Pairs {
		ranked := results[p.Query]
		r1, r5, rr := RecallAtK(ranked, p.Expect, 1), RecallAtK(ranked, p.Expect, 5), ReciprocalRank(ranked, p.Expect)
		m.Recall1 += r1
		m.Recall5 += r5
		m.MRR += rr
		k := m.PerKind[p.Kind]
		k.Pairs++
		k.Recall1 += r1
		k.Recall5 += r5
		k.MRR += rr
		m.PerKind[p.Kind] = k
	}
	if m.Pairs > 0 {
		n := float64(m.Pairs)
		m.Recall1 /= n
		m.Recall5 /= n
		m.MRR /= n
	}
	for name, k := range m.PerKind {
		n := float64(k.Pairs)
		k.Recall1 /= n
		k.Recall5 /= n
		k.MRR /= n
		m.PerKind[name] = k
	}
	return m
}

type PairOutcome struct {
	Pair Pair
	Rank int      // rank 1-based expect pertama di top TopK; 0 = tidak ada
	Got  []string // top 5 path hasil (untuk log)
}

// Failures mengembalikan pasangan dengan recall@5 < 1, urut sesuai golden.
func (g Golden) Failures(results map[string][]string) []PairOutcome {
	var out []PairOutcome
	for _, p := range g.Pairs {
		ranked := results[p.Query]
		if RecallAtK(ranked, p.Expect, 5) >= 1 {
			continue
		}
		got := ranked
		if len(got) > 5 {
			got = got[:5]
		}
		out = append(out, PairOutcome{Pair: p, Rank: firstRank(ranked, p.Expect), Got: append([]string{}, got...)})
	}
	return out
}
