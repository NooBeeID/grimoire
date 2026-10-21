package rankeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Baseline memakai skema yang sama dengan Metrics.
type Baseline = Metrics

func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// SaveBaseline menulis JSON deterministik (indent 2, per_kind urut Kinds(), 4 desimal).
func SaveBaseline(path string, m Metrics) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "{\n  \"pairs\": %d,\n  \"recall_at_1\": %s,\n  \"recall_at_5\": %s,\n  \"mrr\": %s,\n  \"per_kind\": {",
		m.Pairs, f4(m.Recall1), f4(m.Recall5), f4(m.MRR))
	first := true
	for _, k := range Kinds() {
		km, ok := m.PerKind[k]
		if !ok {
			continue
		}
		if !first {
			buf.WriteString(",")
		}
		first = false
		fmt.Fprintf(&buf, "\n    %q: {\n      \"pairs\": %d,\n      \"recall_at_1\": %s,\n      \"recall_at_5\": %s,\n      \"mrr\": %s\n    }",
			k, km.Pairs, f4(km.Recall1), f4(km.Recall5), f4(km.MRR))
	}
	if !first {
		buf.WriteString("\n  ")
	}
	buf.WriteString("}\n}\n")
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func f4(v float64) string {
	b, _ := json.Marshal(round4(v))
	return string(b)
}

// LoadBaseline membaca baseline; error bila file tak ada (dibungkus %w) atau Pairs <= 0.
func LoadBaseline(path string) (Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, fmt.Errorf("baseline: %w", err)
	}
	var m Baseline
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Baseline{}, fmt.Errorf("baseline: %w", err)
	}
	if m.Pairs <= 0 {
		return Baseline{}, fmt.Errorf("baseline: pairs harus > 0 (dapat %d)", m.Pairs)
	}
	return m, nil
}
