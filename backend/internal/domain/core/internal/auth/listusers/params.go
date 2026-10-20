package listusers

import (
	"net/url"
	"strconv"

	"grimoire/internal/domain/core/internal/auth/base"
)

// parseQuery mengubah query string menjadi UserFilterInput. Hanya memeriksa BENTUK angka;
// rule nilai ada di base.NewUserFilter. Parameter kosong = tidak dikirim.
func parseQuery(q url.Values) (base.UserFilterInput, base.FieldErrors) {
	in := base.UserFilterInput{Role: q.Get("role"), Status: q.Get("status"), Query: q.Get("q")}
	fe := base.FieldErrors{}
	in.Limit = parseInt(q, "limit", fe)
	in.Offset = parseInt(q, "offset", fe)
	return in, fe
}

func parseInt(q url.Values, key string, fe base.FieldErrors) *int {
	raw := q.Get(key)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		fe[key] = "harus berupa angka bulat"
		return nil
	}
	return &n
}
