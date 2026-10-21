package listprojects

import (
	"net/url"
	"strconv"

	"grimoire/internal/domain/project/internal/management/base"
)

// parseQuery hanya memeriksa BENTUK angka. Tidak ada rule nilai.
func parseQuery(q url.Values) (ProjectFilterInput, base.FieldErrors) {
	in := ProjectFilterInput{Query: q.Get("q")}
	fe := base.FieldErrors{}
	parse := func(key string) *int {
		s := q.Get(key)
		if s == "" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			fe[key] = "harus berupa angka bulat"
			return nil
		}
		return &n
	}
	in.Limit = parse("limit")
	in.Offset = parse("offset")
	return in, fe
}
