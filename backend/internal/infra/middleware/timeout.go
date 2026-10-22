package middleware

import (
	"context"
	"net/http"
	"time"
)

type baseCtxKey struct{}

// Timeout memasang deadline pada ctx request. Handler yang menerima DeadlineExceeded
// memanggil WriteError sehingga menjadi 504 TIMEOUT. ctx sebelum deadline disimpan agar
// ExtendTimeout dapat memperpanjangnya per route.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		panic("httpx: Timeout membutuhkan d > 0")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			base := r.Context()
			ctx, cancel := context.WithTimeout(context.WithValue(base, baseCtxKey{}, base), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extendedCtx: deadline/pembatalan dari dl, nilai dari values (ctx request saat ini).
type extendedCtx struct {
	values context.Context
	dl     context.Context
}

func (c extendedCtx) Deadline() (time.Time, bool) { return c.dl.Deadline() }
func (c extendedCtx) Done() <-chan struct{}       { return c.dl.Done() }
func (c extendedCtx) Err() error                  { return c.dl.Err() }
func (c extendedCtx) Value(k any) any             { return c.values.Value(k) }

// ExtendTimeout mengganti deadline Timeout global dengan d untuk route ini. Nilai ctx
// (Session, TraceID) yang dipasang setelah Timeout tetap terbaca. Pembatalan klien
// merambat karena deadline baru diturunkan dari ctx sebelum Timeout.
func ExtendTimeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		panic("httpx: ExtendTimeout membutuhkan d > 0")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			base := r.Context()
			if b, ok := base.Value(baseCtxKey{}).(context.Context); ok {
				base = b
			}
			dl, cancel := context.WithTimeout(base, d)
			defer cancel()
			ctx := extendedCtx{values: r.Context(), dl: dl}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
