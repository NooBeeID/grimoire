package openapi

import (
	"sort"
	"strings"
)

// Candidate adalah satu operasi yang dibandingkan oleh Similar.
type Candidate struct{ Method, Path, OperationID, Summary string }

const similarMaxRunes = 200

func clip(s string) []rune {
	r := []rune(strings.ToLower(s))
	if len(r) > similarMaxRunes {
		r = r[:similarMaxRunes]
	}
	return r
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// Similar mengurutkan kandidat menurut kemiripan: skor = min(Levenshtein("method path"), Levenshtein(operationId))
// pada huruf kecil, string dipotong 200 rune. Seri dipecah dengan urutan masukan (kanonik). Maksimal n.
// Hasil tidak pernah nil.
func Similar(cands []Candidate, method, path, operationID string, n int) []Candidate {
	type scored struct {
		c     Candidate
		score int
		idx   int
	}
	q := clip(strings.TrimSpace(method + " " + path))
	useQ := method != "" || path != ""
	qid := clip(operationID)
	rs := make([]scored, 0, len(cands))
	for i, c := range cands {
		score := 1 << 30
		if useQ {
			score = min(score, levenshtein(clip(c.Method+" "+c.Path), q))
		}
		if operationID != "" && c.OperationID != "" {
			score = min(score, levenshtein(clip(c.OperationID), qid))
		}
		rs = append(rs, scored{c, score, i})
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].score != rs[j].score {
			return rs[i].score < rs[j].score
		}
		return rs[i].idx < rs[j].idx
	})
	if n < 0 {
		n = 0
	}
	if len(rs) > n {
		rs = rs[:n]
	}
	out := make([]Candidate, len(rs))
	for i, r := range rs {
		out[i] = r.c
	}
	return out
}
