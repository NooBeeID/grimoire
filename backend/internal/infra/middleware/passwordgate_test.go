package middleware

import (
	"net/http"
	"testing"

	"grimoire/lib/common/reqctx"
)

func gateWith(id *reqctx.Identity) http.Handler {
	exempt := map[string]struct{}{"GET /api/auth/me": {}, "POST /api/auth/change-password": {}}
	g := PasswordChangeGate(exempt)(http.HandlerFunc(okHandler))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id != nil {
			r = r.WithContext(reqctx.WithIdentity(r.Context(), *id))
		}
		g.ServeHTTP(w, r)
	})
}

func TestPasswordGate_BlocksWhenFlagged(t *testing.T) {
	useLogger(t)
	h := gateWith(&reqctx.Identity{UserID: "u", Role: "super_admin", MustChangePassword: true})
	w := do(h, "GET", "/api/users", nil, "")
	if w.Code != 403 || decodeErr(t, w).Error.Code != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestPasswordGate_AllowsExempt(t *testing.T) {
	useLogger(t)
	h := gateWith(&reqctx.Identity{UserID: "u", MustChangePassword: true})
	if w := do(h, "GET", "/api/auth/me", nil, ""); w.Code != 200 {
		t.Fatalf("me: %d", w.Code)
	}
	if w := do(h, "POST", "/api/auth/change-password", nil, ""); w.Code != 200 {
		t.Fatalf("change-password: %d", w.Code)
	}
}

func TestPasswordGate_AnonymousPasses(t *testing.T) {
	useLogger(t)
	if w := do(gateWith(nil), "GET", "/api/users", nil, ""); w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestPasswordGate_UnflaggedPasses(t *testing.T) {
	useLogger(t)
	h := gateWith(&reqctx.Identity{UserID: "u"})
	if w := do(h, "GET", "/api/users", nil, ""); w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestPasswordGate_MethodMatters(t *testing.T) {
	useLogger(t)
	h := gateWith(&reqctx.Identity{UserID: "u", MustChangePassword: true})
	if w := do(h, "GET", "/api/auth/change-password", nil, ""); w.Code != 403 {
		t.Fatalf("got %d", w.Code)
	}
}
