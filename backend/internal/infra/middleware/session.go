package middleware

import (
	"grimoire/internal/infra/http/respond"
	"log/slog"
	"net/http"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

// SessionCookieName adalah nama cookie sesi (dibuat oleh module auth).
const SessionCookieName = "grimoire_session"

// Session me-resolve cookie sesi lewat bus. Tanpa cookie, request lanjut anonim tanpa
// memanggil bus. Sesi tidak valid menghapus cookie dan lanjut anonim.
func Session(b *bus.Registry, cookieSecure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookieName)
			if err != nil || c.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			res, err := bus.CallTyped[resolvesession.Response](ctx, b,
				constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{Token: c.Value})
			if err != nil {
				slog.ErrorContext(ctx, "resolve session gagal", "error", err)
				respond.WriteError(w, r, errs.New(errs.CodeInternal, ""))
				return
			}
			if !res.Found {
				http.SetCookie(w, &http.Cookie{
					Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1,
					HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: cookieSecure,
				})
				next.ServeHTTP(w, r)
				return
			}
			if res.UserID == "" {
				slog.ErrorContext(ctx, "resolve session melanggar kontrak: Found tanpa UserID")
				respond.WriteError(w, r, errs.New(errs.CodeInternal, ""))
				return
			}
			ctx = reqctx.WithIdentity(ctx, reqctx.Identity{
				UserID: res.UserID, Email: res.Email, Name: res.Name,
				Role: res.Role, MustChangePassword: res.MustChangePassword,
			})
			setLogUser(ctx, res.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
