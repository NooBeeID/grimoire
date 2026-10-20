//go:build integration

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/domain/core/internal/auth/login"
	"grimoire/internal/domain/core/internal/auth/resolvesession"
	"grimoire/internal/domain/core/internal/auth/updateuser"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/migrations"
)

const uuPW = "rahasia-panjang-1"

type uuBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *uuBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *uuBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

type uuEnv struct {
	t      *testing.T
	d      *db.DB
	h      http.Handler
	logs   *uuBuf
	hash   string
	mu     sync.Mutex
	audits []record.Request
}

func setupUU(t *testing.T, noAudit bool) *uuEnv {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := testConfig()
	cfg.CookieSecure = false
	cfg.HTTPDefaultBodyBytes = 1 << 20
	cfg.HTTPDefaultTimeout = 30 * time.Second
	cfg.LoginRatePerMinute = 6000
	cfg.SessionAbsoluteTimeout = 168 * time.Hour
	cfg.SessionIdleTimeout = 12 * time.Hour
	logs := &uuBuf{}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	b := bus.NewRegistry(log)
	e := &uuEnv{t: t, d: d, logs: logs}
	if !noAudit {
		b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
			r, err := bus.Expect[record.Request](req)
			if err != nil {
				return nil, err
			}
			e.mu.Lock()
			e.audits = append(e.audits, r)
			e.mu.Unlock()
			return record.Response{Recorded: true}, nil
		})
	}
	root, api, mcp := httpx.NewRouter(cfg, log, b, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	deps := app.Deps{Ctx: ctx, Config: cfg, Log: log, DB: d, Bus: b, API: api, MCP: mcp}
	registerCodes()
	shared, err := buildShared(cfg)
	if err != nil {
		t.Fatal(err)
	}
	resolvesession.Init(deps, shared)
	login.Init(deps, shared)
	updateuser.Init(deps, shared)
	// Stand-in route khusus super_admin: membuktikan role dibaca ulang tiap request.
	api.With(middleware.RequireAuth(), middleware.RequireRole(base.RoleSuperAdmin)).
		Get("/_admin-only", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	e.h = root
	h := base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: cfg.Argon2MemoryKiB, Iterations: 1, Parallelism: 1}}
	if e.hash, err = h.Hash(uuPW); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *uuEnv) seed(email, role string, mustChange bool) string {
	e.t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, must_change_password)
		VALUES ($1,'Nama',$2,$3,$4) RETURNING id::text`, email, e.hash, role, mustChange); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *uuEnv) do(method, path, body string, cookie *http.Cookie, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:4000"
	r.Header.Set("X-Requested-With", "grimoire")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "TestUA")
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *uuEnv) login(email string) *http.Cookie {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": uuPW})
	w := e.do(http.MethodPost, "/api/auth/login", string(b), nil, nil)
	if w.Code != 200 {
		e.t.Fatalf("login %s: %d %s", email, w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "grimoire_session" {
			return &http.Cookie{Name: c.Name, Value: c.Value}
		}
	}
	e.t.Fatal("tanpa cookie sesi")
	return nil
}

func (e *uuEnv) patch(id, body string, c *http.Cookie) *httptest.ResponseRecorder {
	return e.do(http.MethodPatch, "/api/users/"+id, body, c, nil)
}

func (e *uuEnv) auditsFor(action string) []record.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []record.Request
	for _, a := range e.audits {
		if a.Action == action {
			out = append(out, a)
		}
	}
	return out
}

func (e *uuEnv) roleOf(id string) string {
	var r string
	if err := e.d.GetContext(context.Background(), &r, `SELECT role FROM users WHERE id=$1::uuid`, id); err != nil {
		e.t.Fatal(err)
	}
	return r
}

type uuErr struct {
	Error struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func uuDecodeErr(t *testing.T, w *httptest.ResponseRecorder) uuErr {
	t.Helper()
	var b uuErr
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("body %s: %v", w.Body, err)
	}
	return b
}

func TestT01_AC6_UpdateUser_CannotDemoteSelf(t *testing.T) {
	e := setupUU(t, false)
	id := e.seed("a@tim.id", base.RoleSuperAdmin, false)
	e.seed("b@tim.id", base.RoleSuperAdmin, false)
	w := e.patch(id, `{"role":"developer"}`, e.login("a@tim.id"))
	b := uuDecodeErr(t, w)
	if w.Code != 403 || b.Error.Details["reason"] != "SELF_ACTION_FORBIDDEN" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if e.roleOf(id) != base.RoleSuperAdmin || len(e.auditsFor("user.updated")) != 0 {
		t.Fatal("role/audit berubah")
	}
}

func TestT01_AC6_UpdateUser_LastAdminRace(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("iter%d", i), func(t *testing.T) {
			e := setupUU(t, false)
			a := e.seed("a@tim.id", base.RoleSuperAdmin, false)
			b := e.seed("b@tim.id", base.RoleSuperAdmin, false)
			ca, cb := e.login("a@tim.id"), e.login("b@tim.id")
			start := make(chan struct{})
			var wg sync.WaitGroup
			res := make([]*httptest.ResponseRecorder, 2)
			wg.Add(2)
			go func() { defer wg.Done(); <-start; res[0] = e.patch(b, `{"role":"developer"}`, ca) }()
			go func() { defer wg.Done(); <-start; res[1] = e.patch(a, `{"role":"developer"}`, cb) }()
			close(start)
			wg.Wait()
			// Pemenang 200. Pecundang: 409 LAST_SUPER_ADMIN bila lolos RequireRole sebelum commit
			// pemenang, atau 403 FORBIDDEN bila rolenya sudah turun saat request dibaca.
			ok, lost := 0, 0
			for _, w := range res {
				switch w.Code {
				case 200:
					ok++
				case 409:
					if uuDecodeErr(t, w).Error.Details["reason"] != "LAST_SUPER_ADMIN" {
						t.Fatalf("409 tanpa LAST_SUPER_ADMIN: %s", w.Body)
					}
					lost++
				case 403:
					lost++
				default:
					t.Fatalf("status tak terduga %d %s", w.Code, w.Body)
				}
			}
			if ok != 1 || lost != 1 {
				t.Fatalf("ok=%d lost=%d", ok, lost)
			}
			var n int
			_ = e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM users WHERE role='super_admin' AND is_active`)
			if n != 1 {
				t.Fatalf("super admin aktif=%d", n)
			}
		})
	}
}

func TestT01_AC7_UpdateUser_DeveloperForbidden(t *testing.T) {
	e := setupUU(t, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	w := e.patch(dev, `{"role":"super_admin"}`, e.login("d@tim.id"))
	if w.Code != 403 || uuDecodeErr(t, w).Error.Code != "FORBIDDEN" || e.roleOf(dev) != base.RoleDeveloper {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_AC4_UpdateUser_PasswordGate(t *testing.T) {
	e := setupUU(t, false)
	id := e.seed("a@tim.id", base.RoleSuperAdmin, true)
	w := e.patch(id, `{"name":"Baru"}`, e.login("a@tim.id"))
	if w.Code != 403 || uuDecodeErr(t, w).Error.Code != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_UpdateUser_RoleChangeTakesEffectWithoutRelogin(t *testing.T) {
	e := setupUU(t, false)
	e.seed("a@tim.id", base.RoleSuperAdmin, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	ca, cd := e.login("a@tim.id"), e.login("d@tim.id")
	if w := e.do(http.MethodGet, "/api/_admin-only", "", cd, nil); w.Code != 403 {
		t.Fatalf("sebelum: %d", w.Code)
	}
	var before int
	_ = e.d.GetContext(context.Background(), &before, `SELECT count(*) FROM sessions`)
	if w := e.patch(dev, `{"role":"super_admin"}`, ca); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := e.do(http.MethodGet, "/api/_admin-only", "", cd, nil); w.Code != 200 {
		t.Fatalf("sesudah: %d %s", w.Code, w.Body)
	}
	var after int
	_ = e.d.GetContext(context.Background(), &after, `SELECT count(*) FROM sessions`)
	if before != after {
		t.Fatalf("sesi berubah %d -> %d", before, after)
	}
}

func TestT01_UpdateUser_EmailChangeThenLoginWithNewEmail(t *testing.T) {
	e := setupUU(t, false)
	e.seed("a@tim.id", base.RoleSuperAdmin, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	if w := e.patch(dev, `{"email":"Baru@Tim.ID"}`, e.login("a@tim.id")); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	b, _ := json.Marshal(map[string]string{"email": "d@tim.id", "password": uuPW})
	if w := e.do(http.MethodPost, "/api/auth/login", string(b), nil, nil); w.Code != 401 {
		t.Fatalf("email lama: %d", w.Code)
	}
	e.login("baru@tim.id")
}

func TestT01_UpdateUser_NoCSRFHeader(t *testing.T) {
	e := setupUU(t, false)
	id := e.seed("a@tim.id", base.RoleSuperAdmin, false)
	c := e.login("a@tim.id")
	w := e.do(http.MethodPatch, "/api/users/"+id, `{"name":"Baru"}`, c, map[string]string{"X-Requested-With": ""})
	if w.Code != 403 || uuDecodeErr(t, w).Error.Code != "CSRF_REQUIRED" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT01_UpdateUser_Unauthenticated(t *testing.T) {
	e := setupUU(t, false)
	id := e.seed("a@tim.id", base.RoleSuperAdmin, false)
	if w := e.patch(id, `{"name":"Baru"}`, nil); w.Code != 401 {
		t.Fatalf("%d", w.Code)
	}
}

func TestT01_AC9_UpdateUser_OneAuditEntry(t *testing.T) {
	e := setupUU(t, false)
	admin := e.seed("a@tim.id", base.RoleSuperAdmin, false)
	e.seed("other@tim.id", base.RoleDeveloper, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	c := e.login("a@tim.id")
	count := func() int { return len(e.auditsFor("user.updated")) }

	if w := e.patch(dev, `{"name":"Nama Baru"}`, c); w.Code != 200 || count() != 1 {
		t.Fatalf("(a) %d count=%d", w.Code, count())
	}
	if w := e.patch(dev, `{"name":"Lagi","email":"x@tim.id","role":"super_admin"}`, c); w.Code != 200 || count() != 2 {
		t.Fatalf("(b) %d count=%d", w.Code, count())
	}
	last := e.auditsFor("user.updated")[1]
	fields, _ := json.Marshal(last.Args["changed_fields"])
	if string(fields) != `["name","email","role"]` || last.Args["role_from"] != "developer" || last.Args["role_to"] != "super_admin" ||
		last.Target != dev || last.ActorID != admin || last.Status != "ok" || last.IP == "" {
		t.Fatalf("%+v", last)
	}
	if w := e.patch(dev, `{"name":"Lagi","email":"X@tim.id"}`, c); w.Code != 200 || count() != 2 {
		t.Fatalf("(c) no-op %d count=%d", w.Code, count())
	}
	rejected := []string{
		e.patch(dev, `{"email":"other@tim.id"}`, c).Body.String(),                        // 409
		e.patch(admin, `{"role":"developer"}`, c).Body.String(),                          // 403
		e.patch(dev, `{"name":" "}`, c).Body.String(),                                    // 400
		e.patch("00000000-0000-4000-8000-000000000000", `{"name":"x"}`, c).Body.String(), // 404
	}
	_ = rejected
	if count() != 2 {
		t.Fatalf("(d) count=%d", count())
	}
}

func TestT01_UpdateUser_NegativeStatuses(t *testing.T) {
	e := setupUU(t, false)
	e.seed("a@tim.id", base.RoleSuperAdmin, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	e.seed("other@tim.id", base.RoleDeveloper, false)
	c := e.login("a@tim.id")
	cases := []struct {
		id, body string
		code     int
	}{
		{dev, `{"email":"OTHER@tim.id"}`, 409},
		{dev, `{"name":" ","email":"a@","role":"owner"}`, 400},
		{dev, `{}`, 400},
		{dev, `{"is_active":false}`, 400},
		{"tidak-ada", `{"name":"x"}`, 404},
		{"00000000-0000-4000-8000-000000000000", `{"name":"x"}`, 404},
	}
	for _, tc := range cases {
		if w := e.patch(tc.id, tc.body, c); w.Code != tc.code {
			t.Errorf("%s %s: %d %s", tc.id, tc.body, w.Code, w.Body)
		}
	}
}

func TestT01_AC8_UpdateUser_NothingSensitiveInLogs(t *testing.T) {
	e := setupUU(t, false)
	e.seed("a@tim.id", base.RoleSuperAdmin, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	c := e.login("a@tim.id")
	if w := e.patch(dev, `{"name":"Baru","role":"super_admin"}`, c); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	blob, _ := json.Marshal(e.audits)
	all := e.logs.String() + string(blob)
	for _, bad := range []string{uuPW, "$argon2", e.hash, c.Value} {
		if strings.Contains(all, bad) {
			t.Fatalf("data sensitif bocor: %q", bad)
		}
	}
}

func TestAudit_UnregisteredDoesNotFailUpdate(t *testing.T) {
	e := setupUU(t, true)
	e.seed("a@tim.id", base.RoleSuperAdmin, false)
	dev := e.seed("d@tim.id", base.RoleDeveloper, false)
	if w := e.patch(dev, `{"name":"Baru"}`, e.login("a@tim.id")); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(e.logs.String(), "audit record gagal") {
		t.Fatal("log kegagalan audit tidak ada")
	}
}

// Tanpa lapisan middleware: kedua transaksi benar-benar berebut kunci, sehingga pecundang
// pasti mendapat 409 LAST_SUPER_ADMIN dan tidak pernah deadlock (40P01 -> 500).
type uuNopRec struct{}

func (uuNopRec) Record(context.Context, audit.Event) {}

func TestT01_AC6_UpdateUser_LastAdminRace_Usecase(t *testing.T) {
	for i := 0; i < 20; i++ {
		e := setupUU(t, false)
		a := e.seed("a@tim.id", base.RoleSuperAdmin, false)
		b := e.seed("b@tim.id", base.RoleSuperAdmin, false)
		uc := updateuser.NewUsecase(updateuser.NewRepository(e.d), uuNopRec{}, base.SystemClock)
		dev := "developer"
		run := func(actor, target string) error {
			ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: actor, Role: base.RoleSuperAdmin})
			_, err := uc.UpdateUser(ctx, updateuser.Params{ID: target, Role: &dev})
			return err
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		out := make([]error, 2)
		wg.Add(2)
		go func() { defer wg.Done(); <-start; out[0] = run(a, b) }()
		go func() { defer wg.Done(); <-start; out[1] = run(b, a) }()
		close(start)
		wg.Wait()
		ok, last := 0, 0
		for _, err := range out {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, base.ErrLastSuperAdmin):
				last++
			default:
				t.Fatalf("iter %d: error tak terduga: %v", i, err)
			}
		}
		if ok != 1 || last != 1 {
			t.Fatalf("iter %d: ok=%d last=%d", i, ok, last)
		}
		var n int
		_ = e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM users WHERE role='super_admin' AND is_active`)
		if n != 1 {
			t.Fatalf("super admin aktif=%d", n)
		}
	}
}
