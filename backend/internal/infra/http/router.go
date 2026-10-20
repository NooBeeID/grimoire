package httpx

import (
	"grimoire/internal/infra/middleware"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/common/errs"
)

func notFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, errs.New(errs.CodeNotFound, "resource tidak ditemukan"))
}

// allowedMethods menghitung method yang cocok untuk path pada router rt.
func allowedMethods(rt chi.Routes, r *http.Request) string {
	path := r.URL.Path
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePath != "" {
		path = rc.RoutePath
	}
	var out []string
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if rt.Match(chi.NewRouteContext(), m, path) {
			out = append(out, m)
		}
	}
	return strings.Join(out, ", ")
}

func methodNotAllowed(rt func() chi.Routes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a := allowedMethods(rt(), r); a != "" {
			w.Header().Set("Allow", a)
		}
		WriteError(w, r, errs.New(errs.CodeMethodNotAllowed, "method tidak diizinkan"))
	}
}

// NewRouter merakit router: rantai root, /healthz, /readyz, grup /api (timeout, CSRF,
// session via bus) dan grup /mcp (tanpa CSRF dan session).
func NewRouter(cfg config.Config, log *slog.Logger, b *bus.Registry, p Pinger) (root, api, mcp chi.Router) {
	root = chi.NewRouter()
	root.Use(middleware.Recover(log), middleware.TraceID(), middleware.AccessLog(log), middleware.SecurityHeaders(cfg), middleware.BodyLimit(cfg.HTTPDefaultBodyBytes))
	root.Get("/healthz", healthz)
	root.Head("/healthz", healthz)
	root.Get("/readyz", readyz(p))
	root.Head("/readyz", readyz(p))

	api = chi.NewRouter()
	api.Use(middleware.Timeout(cfg.HTTPDefaultTimeout), middleware.CSRF(), middleware.Session(b, cfg.CookieSecure))
	api.NotFound(notFound)
	api.MethodNotAllowed(methodNotAllowed(func() chi.Routes { return api }))
	root.Mount("/api", api)

	mcp = chi.NewRouter()
	mcp.NotFound(notFound)
	mcp.MethodNotAllowed(methodNotAllowed(func() chi.Routes { return mcp }))
	root.Mount("/mcp", mcp)

	root.NotFound(notFound)
	root.MethodNotAllowed(methodNotAllowed(func() chi.Routes { return root }))
	return root, api, mcp
}
