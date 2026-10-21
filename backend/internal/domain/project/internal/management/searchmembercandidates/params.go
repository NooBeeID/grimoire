package searchmembercandidates

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"grimoire/internal/domain/project/internal/management/base"
)

const (
	defaultLimit  = 10
	maxLimit      = 50 // sama dengan finddevelopers.MaxLimit
	maxQueryRunes = 100
)

type query struct {
	Q     string
	Limit int
}

// parseQuery: q di-trim (maks 100 rune), limit kosong -> 10, integer 1..50.
func parseQuery(rawQ, rawLimit string) (query, error) {
	fe := base.FieldErrors{}
	q := strings.TrimSpace(rawQ)
	if utf8.RuneCountInString(q) > maxQueryRunes {
		fe["q"] = "q maksimal 100 karakter"
	}
	limit := defaultLimit
	if s := strings.TrimSpace(rawLimit); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			fe["limit"] = "limit harus 1-50"
		} else {
			limit = n
		}
	}
	if len(fe) > 0 {
		return query{}, base.InvalidFields(fe)
	}
	return query{Q: q, Limit: limit}, nil
}
