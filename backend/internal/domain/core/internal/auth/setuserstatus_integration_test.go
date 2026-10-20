//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/setuserstatus"
)

func setupStatusEnv(t *testing.T, o envOpts) *loginEnv {
	t.Helper()
	e := setupLogin(t, o)
	shared, err := buildShared(e.cfg.Config)
	if err != nil {
		t.Fatal(err)
	}
	shared.Clock = func() time.Time { return *e.now }
	setuserstatus.Init(e.cfg, shared)
	return e
}

func (e *loginEnv) stSeedRole(email, role string, active, mustChange bool) string {
	e.t.Helper()
	id := e.seed(email, active, mustChange)
	if _, err := e.d.ExecContext(context.Background(), `UPDATE users SET role=$2 WHERE id=$1`, id, role); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// stLoginCookie login dan mengembalikan cookie sesi.
func (e *loginEnv) stLoginCookie(email string) *http.Cookie {
	e.t.Helper()
	w := e.login(email, loginPW)
	c := sessionCookie(w)
	if w.Code != 200 || c == nil {
		e.t.Fatalf("login %s: %d %s", email, w.Code, w.Body)
	}
	return c
}

func (e *loginEnv) stDo(method, path string, c *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("X-Requested-With", "grimoire")
	r.Header.Set("User-Agent", "TestUA")
	r.RemoteAddr = "10.0.0.9:4000"
	if c != nil {
		r.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *loginEnv) stErrReason(w *httptest.ResponseRecorder) string {
	var b struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	s, _ := b.Error.Details["reason"].(string)
	return s
}

func (e *loginEnv) stIsActive(id string) bool {
	e.t.Helper()
	var a bool
	if err := e.d.GetContext(context.Background(), &a, `SELECT is_active FROM users WHERE id=$1`, id); err != nil {
		e.t.Fatal(err)
	}
	return a
}

func TestT01_AC5_DeactivateKillsSessionImmediately(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	admin := e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	b := e.seed("b@tim.id", true, false)
	ac, bc := e.stLoginCookie("admin@tim.id"), e.stLoginCookie("b@tim.id")
	e.stLoginCookie("b@tim.id") // sesi kedua
	if w := e.stDo("GET", "/api/_whoami", bc); w.Code != 200 {
		t.Fatalf("whoami awal %d", w.Code)
	}
	w := e.stDo("POST", "/api/users/"+b+"/deactivate", ac)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var dto base.UserDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil || dto.IsActive || dto.ID != b {
		t.Fatalf("%+v %v", dto, err)
	}
	w = e.stDo("GET", "/api/_whoami", bc)
	if w.Code != 401 {
		t.Fatalf("cookie lama harus 401, dapat %d", w.Code)
	}
	if c := sessionCookie(w); c != nil && c.MaxAge >= 0 && c.Value != "" {
		t.Fatalf("cookie harus dihapus: %+v", c)
	}
	if e.countRows(`SELECT count(*) FROM sessions WHERE user_id=$1`, b) != 0 {
		t.Fatal("sesi masih ada")
	}
	if e.countRows(`SELECT count(*) FROM sessions WHERE user_id=$1`, admin) != 1 {
		t.Fatal("sesi admin ikut terhapus")
	}
	lw := e.login("b@tim.id", loginPW)
	if lw.Code != 401 || errCode(t, lw) != "INVALID_CREDENTIALS" {
		t.Fatalf("%d %s", lw.Code, lw.Body)
	}
}

func TestT01_AC5_ActivateAllowsLoginAgain(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	b := e.seed("b@tim.id", false, false)
	ac := e.stLoginCookie("admin@tim.id")
	if w := e.stDo("POST", "/api/users/"+b+"/activate", ac); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if e.countRows(`SELECT count(*) FROM sessions WHERE user_id=$1`, b) != 0 {
		t.Fatal("activate tidak boleh memulihkan sesi")
	}
	if w := e.login("b@tim.id", loginPW); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_AC6_CannotDeactivateSelf(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	admin := e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	e.stSeedRole("admin2@tim.id", base.RoleSuperAdmin, true, false)
	ac := e.stLoginCookie("admin@tim.id")
	w := e.stDo("POST", "/api/users/"+admin+"/deactivate", ac)
	if w.Code != 403 || errCode(t, w) != "FORBIDDEN" || e.stErrReason(w) != "SELF_ACTION_FORBIDDEN" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !e.stIsActive(admin) || len(e.auditsFor("user.deactivated")) != 0 {
		t.Fatal("state berubah / ada audit")
	}
}

func TestT01_AC6_LastSuperAdminRace(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	a := e.stSeedRole("a@tim.id", base.RoleSuperAdmin, true, false)
	b := e.stSeedRole("b@tim.id", base.RoleSuperAdmin, true, false)
	ac, bc := e.stLoginCookie("a@tim.id"), e.stLoginCookie("b@tim.id")
	sawConflict401 := 0
	defer func() { t.Logf("kalah via 401 (sesi dicabut lebih dulu): %d dari 20", sawConflict401) }()
	for i := 0; i < 20; i++ {
		if _, err := e.d.ExecContext(context.Background(), `UPDATE users SET is_active=true WHERE id IN ($1,$2)`, a, b); err != nil {
			t.Fatal(err)
		}
		ac, bc = e.stLoginIfNeeded("a@tim.id", ac), e.stLoginIfNeeded("b@tim.id", bc)
		codes := make([]*httptest.ResponseRecorder, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); codes[0] = e.stDo("POST", "/api/users/"+b+"/deactivate", ac) }()
		go func() { defer wg.Done(); codes[1] = e.stDo("POST", "/api/users/"+a+"/deactivate", bc) }()
		wg.Wait()
		// Kalah balapan bisa berupa 409 (autentikasi lolos lebih dulu) atau 401 (sesinya sudah
		// dicabut pemenang sebelum request ini terautentikasi); keduanya menjaga invarian.
		ok, conflict := 0, 0
		for _, w := range codes {
			switch {
			case w.Code == 200:
				ok++
			case w.Code == 409 && e.stErrReason(w) == "LAST_SUPER_ADMIN" && errCode(t, w) == "CONFLICT":
				conflict++
			case w.Code == 401:
				conflict++
				sawConflict401++
			default:
				t.Fatalf("iter %d: %d %s", i, w.Code, w.Body)
			}
		}
		if ok != 1 || conflict != 1 || e.countRows(`SELECT count(*) FROM users WHERE role='super_admin' AND is_active`) != 1 {
			t.Fatalf("iter %d: ok=%d conflict=%d", i, ok, conflict)
		}
	}
}

// stLoginIfNeeded login ulang bila cookie lama sudah tidak valid (user sempat dinonaktifkan).
func (e *loginEnv) stLoginIfNeeded(email string, c *http.Cookie) *http.Cookie {
	if w := e.stDo("GET", "/api/_whoami", c); w.Code == 200 {
		return c
	}
	return e.stLoginCookie(email)
}

func TestT01_AC6_DeactivateDeveloperWhileOneAdmin(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	dev := e.seed("dev@tim.id", true, false)
	ac := e.stLoginCookie("admin@tim.id")
	if w := e.stDo("POST", "/api/users/"+dev+"/deactivate", ac); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_AC7_StatusAccessControl(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	e.stSeedRole("must@tim.id", base.RoleSuperAdmin, true, true)
	e.seed("dev@tim.id", true, false)
	target := e.seed("t@tim.id", true, false)
	dc := e.stLoginCookie("dev@tim.id")
	mc := e.stLoginCookie("must@tim.id")
	for _, act := range []string{"deactivate", "activate"} {
		p := "/api/users/" + target + "/" + act
		if w := e.stDo("POST", p, dc); w.Code != 403 || errCode(t, w) != "FORBIDDEN" {
			t.Fatalf("developer %s: %d %s", act, w.Code, w.Body)
		}
		if w := e.stDo("POST", p, nil); w.Code != 401 {
			t.Fatalf("anonim %s: %d", act, w.Code)
		}
		if w := e.stDo("POST", p, mc); w.Code != 403 || errCode(t, w) != "PASSWORD_CHANGE_REQUIRED" {
			t.Fatalf("must-change %s: %d %s", act, w.Code, w.Body)
		}
	}
	if !e.stIsActive(target) {
		t.Fatal("target berubah")
	}
}

func TestT01_AC9_StatusAuditEntries(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	b := e.seed("b@tim.id", true, false)
	ac := e.stLoginCookie("admin@tim.id")
	e.stLoginCookie("b@tim.id")
	e.stLoginCookie("b@tim.id")
	if w := e.stDo("POST", "/api/users/"+b+"/deactivate", ac); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	got := e.auditsFor("user.deactivated")
	if len(got) != 1 {
		t.Fatalf("%d audit", len(got))
	}
	r := got[0]
	if r.ActorType != "user" || r.ActorLabel != "admin@tim.id" || r.TargetType != "user" || r.Target != b ||
		r.Status != "ok" || r.Args["revoked_sessions"] != int64(2) && r.Args["revoked_sessions"] != float64(2) && r.Args["revoked_sessions"] != 2 {
		t.Fatalf("%+v", r)
	}
	if w := e.stDo("POST", "/api/users/"+b+"/deactivate", ac); w.Code != 200 || len(e.auditsFor("user.deactivated")) != 1 {
		t.Fatalf("no-op: %d", w.Code)
	}
	if w := e.stDo("POST", "/api/users/"+b+"/activate", ac); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if len(e.auditsFor("user.activated")) != 1 {
		t.Fatal("audit activated")
	}
	if w := e.stDo("POST", "/api/users/"+b+"/activate", ac); w.Code != 200 || len(e.auditsFor("user.activated")) != 1 {
		t.Fatal("activate no-op menambah audit")
	}
}

func TestT01_AC8_StatusNoSecrets(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	b := e.seed("b@tim.id", true, false)
	ac := e.stLoginCookie("admin@tim.id")
	bc := e.stLoginCookie("b@tim.id")
	e.stDo("POST", "/api/users/"+b+"/deactivate", ac)
	e.stDo("POST", "/api/users/"+b+"/activate", ac)
	blob := e.logs.String()
	e.mu.Lock()
	for _, a := range e.audits {
		j, _ := json.Marshal(a)
		blob += string(j)
	}
	e.mu.Unlock()
	for _, s := range []string{ac.Value, bc.Value, "$argon2id$"} {
		if strings.Contains(blob, s) {
			t.Fatalf("rahasia bocor: %q", s)
		}
	}
}

func TestStatus_AuditUnregisteredDoesNotFail(t *testing.T) {
	e := setupStatusEnv(t, envOpts{noAudit: true})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	b := e.seed("b@tim.id", true, false)
	ac := e.stLoginCookie("admin@tim.id")
	if w := e.stDo("POST", "/api/users/"+b+"/deactivate", ac); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(e.logs.String(), "audit record gagal") {
		t.Fatal("log audit gagal tidak ada")
	}
}

func TestStatus_InvalidAndUnknownID(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	ac := e.stLoginCookie("admin@tim.id")
	for _, id := range []string{"bukan-uuid", "00000000-0000-0000-0000-000000000000"} {
		for _, act := range []string{"deactivate", "activate"} {
			if w := e.stDo("POST", "/api/users/"+id+"/"+act, ac); w.Code != 404 || errCode(t, w) != "NOT_FOUND" {
				t.Fatalf("%s %s: %d %s", id, act, w.Code, w.Body)
			}
		}
	}
}

func TestStatus_RequiresCSRFHeader(t *testing.T) {
	e := setupStatusEnv(t, envOpts{})
	e.stSeedRole("admin@tim.id", base.RoleSuperAdmin, true, false)
	b := e.seed("b@tim.id", true, false)
	ac := e.stLoginCookie("admin@tim.id")
	r := httptest.NewRequest("POST", "/api/users/"+b+"/deactivate", nil)
	r.AddCookie(&http.Cookie{Name: ac.Name, Value: ac.Value})
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 403 || errCode(t, w) != "CSRF_REQUIRED" || !e.stIsActive(b) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
