package rankeval

import "fmt"

// FormatSummary: "pairs=40 recall@1=0.xxxx recall@5=0.xxxx MRR=0.xxxx".
func FormatSummary(m Metrics) string {
	return fmt.Sprintf("pairs=%d recall@1=%.4f recall@5=%.4f MRR=%.4f", m.Pairs, m.Recall1, m.Recall5, m.MRR)
}

// FormatPerKind mengembalikan satu baris per kind (urut Kinds()), melewati kind tanpa pasangan.
func FormatPerKind(m Metrics) []string {
	var out []string
	for _, k := range Kinds() {
		km, ok := m.PerKind[k]
		if !ok {
			continue
		}
		out = append(out, fmt.Sprintf("%-10s pairs=%d r@1=%.4f r@5=%.4f mrr=%.4f", k, km.Pairs, km.Recall1, km.Recall5, km.MRR))
	}
	return out
}

// FormatDrops: "recall_at_5 turun 0.0300 (baseline 0.7000 -> sekarang 0.6700)".
func FormatDrops(d []Drop) []string {
	out := make([]string, 0, len(d))
	for _, x := range d {
		out = append(out, fmt.Sprintf("%s turun %.4f (baseline %.4f -> sekarang %.4f)", x.Metric, x.Delta, x.Base, x.Cur))
	}
	return out
}
