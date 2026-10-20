package middleware

import (
	"grimoire/internal/infra/http/respond"
	"log/slog"
	"net/http"
	"runtime/debug"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Recover menangkap panic handler dan menjawab 500 generik. Berada paling luar,
// sehingga trace id dibaca dari header respons yang sudah diset TraceID.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := newStatusWriter(w)
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				tid := sw.Header().Get(respond.TraceHeader)
				if tid == "" {
					tid = reqctx.TraceID(r.Context())
				}
				log.ErrorContext(r.Context(), "panic recovered",
					"panic", rec, "stack", string(debug.Stack()),
					"trace_id", tid, "method", r.Method, "path", r.URL.Path)
				if !sw.wroteHeader {
					respond.WriteError(sw, r, errs.New(errs.CodeInternal, ""))
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}
