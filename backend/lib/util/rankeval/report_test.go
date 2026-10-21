package rankeval

import (
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	m := sampleMetrics()
	if got := FormatSummary(m); got != "pairs=5 recall@1=0.1235 recall@5=0.5000 MRR=0.3333" {
		t.Errorf("summary %q", got)
	}
	lines := FormatPerKind(m)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "keyword") || !strings.HasPrefix(lines[1], "phrase") {
		t.Errorf("perkind %q", lines)
	}
	d := FormatDrops([]Drop{{Metric: "recall_at_5", Base: 0.7, Cur: 0.67, Delta: 0.03}})
	if len(d) != 1 || d[0] != "recall_at_5 turun 0.0300 (baseline 0.7000 -> sekarang 0.6700)" {
		t.Errorf("drops %q", d)
	}
}
