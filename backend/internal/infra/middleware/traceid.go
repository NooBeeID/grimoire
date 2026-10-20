package middleware

import (
	"grimoire/internal/infra/http/respond"
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"grimoire/lib/common/reqctx"
)

var traceIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// TraceID memakai X-Request-Id masuk bila valid, selain itu membuat uuid baru.
func TraceID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(respond.TraceHeader)
			if !traceIDRe.MatchString(id) {
				id = uuid.NewString()
			}
			w.Header().Set(respond.TraceHeader, id)
			next.ServeHTTP(w, r.WithContext(reqctx.WithTraceID(r.Context(), id)))
		})
	}
}
