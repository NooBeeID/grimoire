package rankeval

const epsilon = 1e-9

type Drop struct {
	Metric string // "recall_at_1" | "recall_at_5" | "mrr"
	Base   float64
	Cur    float64
	Delta  float64 // Base - Cur, > 0
}

// Compare membandingkan hanya metrik keseluruhan. Turun tepat sebesar tol masih lulus;
// perbaikan tidak pernah dilaporkan. tol < 0 dianggap 0.
func Compare(cur, base Metrics, tol float64) []Drop {
	if tol < 0 {
		tol = 0
	}
	var out []Drop
	check := func(name string, b, c float64) {
		b, c = round4(b), round4(c)
		if d := b - c; d > tol+epsilon {
			out = append(out, Drop{Metric: name, Base: b, Cur: c, Delta: d})
		}
	}
	check("recall_at_1", base.Recall1, cur.Recall1)
	check("recall_at_5", base.Recall5, cur.Recall5)
	check("mrr", base.MRR, cur.MRR)
	return out
}
