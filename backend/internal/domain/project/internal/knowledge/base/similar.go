package base

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// SimilarPaths mengembalikan maksimal n path dari all yang mirip target (target sendiri tidak ikut):
// basename sama (case-insensitive) lebih dulu, lalu jarak Levenshtein <= max(3, len(target)/3). Deterministik.
func SimilarPaths(target string, all []string, n int) []string {
	if n <= 0 {
		return nil
	}
	lt := strings.ToLower(target)
	lbase := strings.ToLower(Base(target))
	maxDist := utf8.RuneCountInString(target) / 3
	if maxDist < 3 {
		maxDist = 3
	}

	var sameBase []string
	type cand struct {
		p string
		d int
	}
	var near []cand
	for _, p := range all {
		if p == target {
			continue
		}
		if strings.ToLower(Base(p)) == lbase {
			sameBase = append(sameBase, p)
			continue
		}
		if d := levenshtein(lt, strings.ToLower(p)); d <= maxDist {
			near = append(near, cand{p, d})
		}
	}
	sort.Strings(sameBase)
	sort.Slice(near, func(i, j int) bool {
		if near[i].d != near[j].d {
			return near[i].d < near[j].d
		}
		return near[i].p < near[j].p
	})
	out := make([]string, 0, n)
	out = append(out, sameBase...)
	for _, c := range near {
		out = append(out, c.p)
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
