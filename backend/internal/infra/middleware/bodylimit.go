package middleware

import (
	"context"
	"io"
	"net/http"
)

type origBodyKey struct{}

// BodyLimit membatasi body secara lazy (http.MaxBytesReader). Batas terdalam menang,
// termasuk menaikkan batas, karena selalu membungkus body asli.
func BodyLimit(n int64) func(http.Handler) http.Handler {
	if n <= 0 {
		panic("httpx: BodyLimit membutuhkan n > 0")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			orig, ok := ctx.Value(origBodyKey{}).(io.ReadCloser)
			if !ok {
				orig = r.Body
				ctx = context.WithValue(ctx, origBodyKey{}, orig)
			}
			r = r.WithContext(ctx)
			r.Body = http.MaxBytesReader(w, orig, n)
			next.ServeHTTP(w, r)
		})
	}
}
