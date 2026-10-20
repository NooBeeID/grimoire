//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/createuser"
	"grimoire/internal/domain/core/internal/auth/getuser"
	"grimoire/internal/domain/core/internal/auth/listusers"
	"grimoire/internal/domain/core/internal/auth/resetpassword"
	"grimoire/internal/domain/core/internal/auth/setuserstatus"
	"grimoire/internal/domain/core/internal/auth/updateuser"
)

const adminPW = "rahasia-admin-123"

// setupUsers menyiapkan env login + semua route /api/users*.
func setupUsers(t *testing.T) *loginEnv {
	t.Helper()
	e := setupLogin(t, envOpts{})
	shared, err := buildShared(e.cfg.Config)
	if err != nil {
		t.Fatal(err)
	}
	shared.Clock = func() time.Time { return *e.now }
	listusers.Init(e.cfg, shared)
	createuser.Init(e.cfg, shared)
	getuser.Init(e.cfg, shared)
	updateuser.Init(e.cfg, shared)
	setuserstatus.Init(e.cfg, shared)
	resetpassword.Init(e.cfg, shared)
	return e
}

// seedRole menyisipkan user dengan role tertentu (password = loginPW).
func (e *loginEnv) seedRole(email, role string, active, mustChange bool) string {
	e.t.Helper()
	id := e.seed(email, active, mustChange)
	if _, err := e.d.ExecContext(context.Background(), `UPDATE users SET role=$1 WHERE id=$2::uuid`, role, id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *loginEnv) do(c *http.Cookie, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader("{}"))
	r.Header.Set("X-Requested-With", "grimoire")
	if c != nil {
		r.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func TestT01_AC7_DeveloperForbiddenOnUsers(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("dev@tim.id", "developer", true, false)
	target := e.seedRole("target@tim.id", "developer", true, false)
	dev := e.loginCookie("dev@tim.id", loginPW)

	routes := []struct{ method, path string }{
		{"GET", "/api/users"},
		{"POST", "/api/users"},
		{"GET", "/api/users/" + target},
		{"PATCH", "/api/users/" + target},
		{"POST", "/api/users/" + target + "/deactivate"},
		{"POST", "/api/users/" + target + "/activate"},
		{"POST", "/api/users/" + target + "/reset-password"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := e.do(dev, rt.method, rt.path)
			if w.Code != 403 || errCode(t, w) != "FORBIDDEN" {
				t.Fatalf("dev: %d %s", w.Code, w.Body)
			}
			if w := e.do(nil, rt.method, rt.path); w.Code != 401 || errCode(t, w) != "UNAUTHORIZED" {
				t.Fatalf("anon: %d %s", w.Code, w.Body)
			}
		})
	}
	// developer tetap tidak melihat data user lain.
	if w := e.do(dev, "GET", "/api/users"); strings.Contains(w.Body.String(), "target@tim.id") {
		t.Fatal("bocor")
	}
}

func TestT01_ListUsers_SuperAdminOK(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("admin@tim.id", "super_admin", true, false)
	e.seedRole("dev@tim.id", "developer", true, false)
	c := e.loginCookie("admin@tim.id", loginPW)
	w := e.do(c, "GET", "/api/users")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var b struct {
		Items  []map[string]any `json:"items"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Limit != 20 || b.Offset != 0 || b.Total != 2 || len(b.Items) != 2 {
		t.Fatalf("%+v", b)
	}
	for _, k := range []string{"id", "email", "name", "role", "is_active", "must_change_password", "created_at", "created_by", "created_by_label", "updated_at", "last_login_at"} {
		if _, ok := b.Items[0][k]; !ok {
			t.Fatalf("kunci %s hilang", k)
		}
	}
	if b.Items[0]["last_login_at"] == nil && b.Items[1]["last_login_at"] == nil {
		t.Fatal("last_login_at admin harus terisi setelah login")
	}
}

func TestT01_AC4_ListUsersBlockedByPasswordGate(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("admin@tim.id", "super_admin", true, true)
	c := e.loginCookie("admin@tim.id", loginPW)
	w := e.do(c, "GET", "/api/users")
	if w.Code != 403 || errCode(t, w) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_ListUsers_ValidationErrors(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("admin@tim.id", "super_admin", true, false)
	c := e.loginCookie("admin@tim.id", loginPW)
	cases := map[string][]string{
		"?status=gone&limit=500": {"status", "limit"},
		"?limit=abc":             {"limit"},
		"?role=admin":            {"role"},
		"?offset=-1":             {"offset"},
	}
	for q, keys := range cases {
		w := e.do(c, "GET", "/api/users"+q)
		var b struct {
			Error struct {
				Code    string `json:"code"`
				Details struct {
					Fields map[string]string `json:"fields"`
				} `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &b)
		if w.Code != 400 || b.Error.Code != "INVALID_ARGUMENT" {
			t.Fatalf("%s: %d %s", q, w.Code, w.Body)
		}
		for _, k := range keys {
			if b.Error.Details.Fields[k] == "" {
				t.Fatalf("%s: field %s hilang: %s", q, k, w.Body)
			}
		}
	}
}

func TestT01_ListUsers_ResponseNeverContainsHash(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("admin@tim.id", "super_admin", true, false)
	e.seedRole("dev@tim.id", "developer", true, false)
	var hash string
	if err := e.d.GetContext(context.Background(), &hash, `SELECT password_hash FROM users WHERE email='dev@tim.id'`); err != nil {
		t.Fatal(err)
	}
	c := e.loginCookie("admin@tim.id", loginPW)
	body := e.do(c, "GET", "/api/users").Body.String()
	for _, bad := range []string{"password_hash", "argon2id", hash} {
		if strings.Contains(body, bad) {
			t.Fatalf("bocor %q", bad)
		}
	}
}

func TestT01_ListUsers_ReflectsDeactivationImmediately(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("admin@tim.id", "super_admin", true, false)
	id := e.seedRole("dev@tim.id", "developer", true, false)
	c := e.loginCookie("admin@tim.id", loginPW)
	if _, err := e.d.ExecContext(context.Background(), `UPDATE users SET is_active=false WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	all := e.do(c, "GET", "/api/users").Body.String()
	if !strings.Contains(all, `"is_active":false`) {
		t.Fatalf("%s", all)
	}
	act := e.do(c, "GET", "/api/users?status=active").Body.String()
	if strings.Contains(act, "dev@tim.id") || !strings.Contains(act, `"total":1`) {
		t.Fatalf("%s", act)
	}
}

func TestT01_ListUsers_NoAuditEntry(t *testing.T) {
	e := setupUsers(t)
	e.seedRole("admin@tim.id", "super_admin", true, false)
	c := e.loginCookie("admin@tim.id", loginPW)
	before := len(e.auditsFor("auth.login.success"))
	total := func() int { e.mu.Lock(); defer e.mu.Unlock(); return len(e.audits) }
	n := total()
	for i := 0; i < 3; i++ {
		e.do(c, "GET", "/api/users")
	}
	if total() != n || len(e.auditsFor("auth.login.success")) != before {
		t.Fatal("GET /api/users tidak boleh menghasilkan audit")
	}
}
