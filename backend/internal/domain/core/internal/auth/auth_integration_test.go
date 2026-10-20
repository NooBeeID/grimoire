//go:build integration

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	httpx "grimoire/internal/infra/http"
	"grimoire/internal/infra/jobs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/migrations"
)

// Wiring lintas feature: memakai auth.Register (15 Init) dan router lengkap, bukan Init per feature.

const wirePW = "rahasia-panjang-1"

type wireEnv struct {
	t      *testing.T
	d      *db.DB
	h      http.Handler
	bus    *bus.Registry
	logs   *syncBuf
	mu     sync.Mutex
	audits []record.Request
}

func setupWire(t *testing.T, noAudit bool) *wireEnv {
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
	logs := &syncBuf{}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	b := bus.NewRegistry(log)
	e := &wireEnv{t: t, d: d, logs: logs}
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
	deps := app.Deps{Ctx: ctx, Config: cfg, Log: log, DB: d, Bus: b, Jobs: jobs.New(d, log), API: api, MCP: mcp}
	Register(deps)
	e.h, e.bus = root, b
	return e
}

func (e *wireEnv) seedAdmin(email string) {
	e.t.Helper()
	h := base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}}
	hash, err := h.Hash(wirePW)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.d.ExecContext(context.Background(), `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ($1,'Admin',$2,'super_admin',true,false)`, email, hash); err != nil {
		e.t.Fatal(err)
	}
}

func (e *wireEnv) do(method, path, body string, c *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:4000"
	r.Header.Set("X-Requested-With", "grimoire")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "TestUA")
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *wireEnv) login(email, pw string) (*http.Cookie, *httptest.ResponseRecorder) {
	b, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	w := e.do(http.MethodPost, "/api/auth/login", string(b), nil)
	if c := sessionCookie(w); c != nil {
		return &http.Cookie{Name: c.Name, Value: c.Value}, w
	}
	return nil, w
}

func (e *wireEnv) auditCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.audits)
}

func (e *wireEnv) auditSnapshot() []record.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]record.Request(nil), e.audits...)
}

func TestRoutesRegistered(t *testing.T) {
	e := setupWire(t, false)
	want := []string{
		"GET /api/auth/me",
		"GET /api/users",
		"GET /api/users/{id}",
		"PATCH /api/users/{id}",
		"POST /api/auth/change-password",
		"POST /api/auth/login",
		"POST /api/auth/logout",
		"POST /api/users",
		"POST /api/users/{id}/activate",
		"POST /api/users/{id}/deactivate",
		"POST /api/users/{id}/reset-password",
	}
	got := map[string]bool{}
	err := chi.Walk(e.h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, w := range want {
		if !got[w] {
			missing = append(missing, w)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("route belum terdaftar: %v", missing)
	}
	// Bus provider T01 terdaftar lewat Register.
	for _, ep := range []string{constants.BUS_CORE_AUTH_RESOLVE_SESSION, constants.BUS_CORE_AUTH_GET_USER, constants.BUS_CORE_AUTH_FIND_DEVELOPERS} {
		if _, err := e.bus.Call(context.Background(), ep, struct{}{}); errors.Is(err, bus.ErrNotRegistered) {
			t.Fatalf("bus %s tidak terdaftar: %v", ep, err)
		}
	}
}

// wireFlow menjalankan 9 aksi audit secara berurutan dan memanggil check setelah tiap langkah.
type wireStep struct {
	action string
	run    func() *httptest.ResponseRecorder
}

func TestT01_AC9_EachActionOneAuditEntry(t *testing.T) {
	e := setupWire(t, false)
	e.seedAdmin("admin@tim.id")
	var ac, tc *http.Cookie
	var tid, temp string
	newPW := "password-baru-123"

	steps := []wireStep{
		{"auth.login.failed", func() *httptest.ResponseRecorder { _, w := e.login("admin@tim.id", "salah-salah-1"); return w }},
		{"auth.login.success", func() *httptest.ResponseRecorder { c, w := e.login("admin@tim.id", wirePW); ac = c; return w }},
		{"user.created", func() *httptest.ResponseRecorder {
			w := e.do("POST", "/api/users", `{"email":"dev@tim.id","name":"Dev","role":"developer"}`, ac)
			var out struct {
				User              struct{ ID string } `json:"user"`
				TemporaryPassword string              `json:"temporary_password"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &out)
			tid, temp = out.User.ID, out.TemporaryPassword
			return w
		}},
		{"user.updated", func() *httptest.ResponseRecorder {
			return e.do("PATCH", "/api/users/"+tid, `{"name":"Dev Baru"}`, ac)
		}},
		{"", func() *httptest.ResponseRecorder { return e.do("PATCH", "/api/users/"+tid, `{"name":"Dev Baru"}`, ac) }}, // no-op
		{"user.deactivated", func() *httptest.ResponseRecorder { return e.do("POST", "/api/users/"+tid+"/deactivate", "", ac) }},
		{"", func() *httptest.ResponseRecorder { return e.do("POST", "/api/users/"+tid+"/deactivate", "", ac) }}, // no-op
		{"user.activated", func() *httptest.ResponseRecorder { return e.do("POST", "/api/users/"+tid+"/activate", "", ac) }},
		{"auth.password.reset", func() *httptest.ResponseRecorder {
			w := e.do("POST", "/api/users/"+tid+"/reset-password", `{}`, ac)
			var out struct {
				TemporaryPassword string `json:"temporary_password"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &out)
			temp = out.TemporaryPassword
			return w
		}},
		{"auth.login.success", func() *httptest.ResponseRecorder { c, w := e.login("dev@tim.id", temp); tc = c; return w }},
		{"auth.password.changed", func() *httptest.ResponseRecorder {
			b, _ := json.Marshal(map[string]string{"current_password": temp, "new_password": newPW})
			return e.do("POST", "/api/auth/change-password", string(b), tc)
		}},
		{"auth.logout", func() *httptest.ResponseRecorder { return e.do("POST", "/api/auth/logout", "", tc) }},
	}
	for i, s := range steps {
		before := e.auditCount()
		w := s.run()
		if w.Code >= 400 && s.action != "auth.login.failed" {
			t.Fatalf("langkah %d (%q): %d %s", i, s.action, w.Code, w.Body)
		}
		after := e.auditSnapshot()
		delta := after[before:]
		if s.action == "" {
			if len(delta) != 0 {
				t.Fatalf("langkah %d no-op menghasilkan audit: %+v", i, delta)
			}
			continue
		}
		if len(delta) != 1 || delta[0].Action != s.action {
			t.Fatalf("langkah %d: want 1 entri %q, got %+v", i, s.action, delta)
		}
	}
}

func TestT01_AC8_PasswordNeverLoggedOrAudited(t *testing.T) {
	e := setupWire(t, false)
	e.seedAdmin("admin@tim.id")
	ac, w := e.login("admin@tim.id", wirePW)
	if ac == nil {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	e.login("admin@tim.id", "salah-rahasia-9")

	given := "password-pilihan-admin-1"
	cw := e.do("POST", "/api/users", `{"email":"a@tim.id","name":"A","role":"developer","password":"`+given+`"}`, ac)
	if cw.Code != 201 && cw.Code != 200 {
		t.Fatalf("create: %d %s", cw.Code, cw.Body)
	}
	gw := e.do("POST", "/api/users", `{"email":"b@tim.id","name":"B","role":"developer"}`, ac)
	var gen struct {
		User              struct{ ID string } `json:"user"`
		TemporaryPassword string              `json:"temporary_password"`
	}
	_ = json.Unmarshal(gw.Body.Bytes(), &gen)
	if gen.TemporaryPassword == "" {
		t.Fatalf("tanpa temporary_password: %s", gw.Body)
	}
	rw := e.do("POST", "/api/users/"+gen.User.ID+"/reset-password", `{}`, ac)
	var rst struct {
		TemporaryPassword string `json:"temporary_password"`
	}
	_ = json.Unmarshal(rw.Body.Bytes(), &rst)
	tc, _ := e.login("b@tim.id", rst.TemporaryPassword)
	newPW := "password-baru-rahasia-7"
	cb, _ := json.Marshal(map[string]string{"current_password": rst.TemporaryPassword, "new_password": newPW})
	e.do("POST", "/api/auth/change-password", string(cb), tc)

	var hashes []string
	if err := e.d.SelectContext(context.Background(), &hashes, `SELECT password_hash FROM users`); err != nil {
		t.Fatal(err)
	}
	ab, _ := json.Marshal(e.auditSnapshot())
	blob := e.logs.String() + string(ab)
	secrets := []string{wirePW, "salah-rahasia-9", given, gen.TemporaryPassword, rst.TemporaryPassword, newPW, ac.Value, tc.Value, "$argon2id$"}
	secrets = append(secrets, hashes...)
	for _, s := range secrets {
		if s != "" && strings.Contains(blob, s) {
			t.Fatalf("rahasia bocor di log/audit: %q", s)
		}
	}
	if bytes.Contains(ab, []byte(`"password"`)) {
		t.Fatalf("kunci password muncul di audit: %s", ab)
	}
}

func TestAudit_UnregisteredDoesNotFailRequest_Wired(t *testing.T) {
	e := setupWire(t, true)
	e.seedAdmin("admin@tim.id")
	ac, w := e.login("admin@tim.id", wirePW)
	if w.Code != 200 || ac == nil {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	if cw := e.do("POST", "/api/users", `{"email":"x@tim.id","name":"X","role":"developer"}`, ac); cw.Code >= 400 {
		t.Fatalf("create: %d %s", cw.Code, cw.Body)
	}
	l := e.logs.String()
	if !strings.Contains(l, "audit record gagal") || !strings.Contains(l, `"trace_id"`) {
		t.Fatalf("log: %s", l)
	}
}
