package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"grimoire/lib/common/reqctx"
)

type logFieldsKey struct{}

// logFields diisi middleware yang lebih dalam (Session) dan dibaca AccessLog.
type logFields struct{ UserID string }

func setLogUser(ctx context.Context, userID string) {
	if lf, ok := ctx.Value(logFieldsKey{}).(*logFields); ok {
		lf.UserID = userID
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AccessLog mencatat satu baris per request. X-Forwarded-For tidak dipercaya.
func AccessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := newStatusWriter(w)
			lf := &logFields{}
			r = r.WithContext(context.WithValue(r.Context(), logFieldsKey{}, lf))

			write := func(status int) {
				route := ""
				if rc := chi.RouteContext(r.Context()); rc != nil {
					route = rc.RoutePattern()
				}
				lvl := slog.LevelInfo
				if status >= 500 {
					lvl = slog.LevelError
				}
				if p := r.URL.Path; p == "/healthz" || p == "/readyz" {
					lvl = slog.LevelDebug
				}
				log.LogAttrs(r.Context(), lvl, "http request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("route", route),
					slog.Int("status", status),
					slog.Int64("bytes", sw.bytes),
					slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
					slog.String("remote_ip", remoteIP(r)),
					slog.String("trace_id", reqctx.TraceID(r.Context())),
					slog.String("user_id", lf.UserID),
				)
			}
			defer func() {
				if rec := recover(); rec != nil {
					write(http.StatusInternalServerError)
					panic(rec)
				}
			}()
			next.ServeHTTP(sw, r)
			write(sw.status)
		})
	}
}
