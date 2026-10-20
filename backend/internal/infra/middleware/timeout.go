package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout memasang deadline pada ctx request. Handler yang menerima DeadlineExceeded
// memanggil WriteError sehingga menjadi 504 TIMEOUT.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		panic("httpx: Timeout membutuhkan d > 0")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
