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
	"grimoire/internal/infra/mailer"
	"grimoire/internal/infra/mailer/mailertest"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	inviteuserc "grimoire/lib/contracts/core/auth/inviteuser"
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
	return setupWireMail(t, noAudit, nil)
}

// setupWireMail seperti setupWire dengan Mailer yang diinjeksi (nil: tidak terkonfigurasi).
func setupWireMail(t *testing.T, noAudit bool, ml mailer.Mailer) *wireEnv {
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
	cfg.InvitationTTL = 168 * time.Hour
	cfg.InvitationMaxPerHour = 20
	cfg.InvitationResendMaxPerHour = 3
	cfg.InvitationPurgeAfter = 720 * time.Hour
	cfg.PublicURL = "https://g.test"
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
	deps := app.Deps{Ctx: ctx, Config: cfg, Log: log, DB: d, Bus: b, Jobs: jobs.New(d, log), API: api, MCP: mcp, Mailer: ml}
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
		"POST /api/users/invite",
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
	for _, ep := range []string{constants.BUS_CORE_AUTH_RESOLVE_SESSION, constants.BUS_CORE_AUTH_GET_USER, constants.BUS_CORE_AUTH_FIND_DEVELOPERS, constants.BUS_CORE_AUTH_INVITE_USER} {
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

// ---- T12 inviteuser ----

func inviteBody(email string) string { return `{"email":"` + email + `"}` }

func (e *wireEnv) adminCookie(email string) *http.Cookie {
	e.t.Helper()
	e.seedAdmin(email)
	c, w := e.login(email, wirePW)
	if c == nil {
		e.t.Fatalf("login admin: %d %s", w.Code, w.Body)
	}
	return c
}

func jsonMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	return m
}

func errCodeOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return jsonMap(t, w)["error"].(map[string]any)["code"].(string)
}

func TestT12_AC6_SuperAdminInvitesFromUsers(t *testing.T) {
	fk := &mailertest.Fake{}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	w := e.do("POST", "/api/users/invite", `{"email":" Baru@Contoh.ID ","name":""}`, ac)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	m := jsonMap(t, w)
	if _, ok := m["credentials"]; ok { // AC10
		t.Fatalf("credentials tidak boleh ada: %s", w.Body)
	}
	if w.Header().Get("Cache-Control") == "no-store" || m["email_sent"] != true || m["created"] != true {
		t.Fatalf("body: %s", w.Body)
	}
	if len(fk.Sent) != 1 || fk.Sent[0].To != "baru@contoh.id" || !strings.Contains(fk.Sent[0].Text, "https://g.test/login") {
		t.Fatalf("sent: %+v", fk.Sent)
	}
	var role string
	var n int
	if err := e.d.GetContext(context.Background(), &role, `SELECT role FROM users WHERE email='baru@contoh.id'`); err != nil || role != "developer" {
		t.Fatalf("role=%q err=%v", role, err)
	}
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM project_members pm JOIN users u ON u.id=pm.user_id WHERE u.email='baru@contoh.id'`); err == nil && n != 0 {
		t.Fatalf("akun tidak boleh punya project: %d", n)
	}
}

func TestT12_AC6_DeveloperForbidden(t *testing.T) {
	fk := &mailertest.Fake{}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	cw := e.do("POST", "/api/users", `{"email":"dev@tim.id","name":"Dev","role":"developer","password":"password-dev-12345"}`, ac)
	if cw.Code != 201 {
		t.Fatalf("create: %d %s", cw.Code, cw.Body)
	}
	dc, _ := e.login("dev@tim.id", "password-dev-12345")
	before := e.auditCount()
	w := e.do("POST", "/api/users/invite", inviteBody("x@tim.id"), dc)
	if w.Code != 403 || len(fk.Sent) != 0 || e.auditCount() != before {
		t.Fatalf("%d %s sent=%d", w.Code, w.Body, len(fk.Sent))
	}
	if aw := e.do("POST", "/api/users/invite", inviteBody("x@tim.id"), nil); aw.Code != 401 && aw.Code != 403 {
		t.Fatalf("anon: %d", aw.Code)
	}
	if n := countUsers(t, e, "x@tim.id"); n != 0 {
		t.Fatalf("akun dibuat: %d", n)
	}
}

func countUsers(t *testing.T, e *wireEnv, email string) int {
	t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM users WHERE email=$1`, email); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestT12_AC4_AlreadyRegisteredAndPendingReissue(t *testing.T) {
	fk := &mailertest.Fake{}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	e.do("POST", "/api/users", `{"email":"dev@tim.id","name":"Dev","role":"developer"}`, ac)
	w := e.do("POST", "/api/users/invite", inviteBody("DEV@tim.id"), ac)
	d := jsonMap(t, w)["error"].(map[string]any)
	if w.Code != 409 || d["code"] != "ALREADY_REGISTERED" || d["details"].(map[string]any)["reason"] != "registered" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// akun menunggu: undang ulang -> 201 created=false, password berganti, email kedua
	if w := e.do("POST", "/api/users/invite", inviteBody("baru@tim.id"), ac); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var h1, h2 string
	_ = e.d.GetContext(context.Background(), &h1, `SELECT password_hash FROM users WHERE email='baru@tim.id'`)
	w = e.do("POST", "/api/users/invite", inviteBody("baru@tim.id"), ac)
	_ = e.d.GetContext(context.Background(), &h2, `SELECT password_hash FROM users WHERE email='baru@tim.id'`)
	if w.Code != 201 || jsonMap(t, w)["created"] != false || h1 == h2 || len(fk.Sent) != 2 {
		t.Fatalf("%d %s sent=%d", w.Code, w.Body, len(fk.Sent))
	}
	// admin (not_developer)
	if w := e.do("POST", "/api/users/invite", inviteBody("admin@tim.id"), ac); w.Code != 409 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT12_AC9_FallbackWhenMailerFails(t *testing.T) {
	fk := &mailertest.Fake{Err: errors.New("smtp mati")}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	w := e.do("POST", "/api/users/invite", inviteBody("f@tim.id"), ac)
	m := jsonMap(t, w)
	c, _ := m["credentials"].(map[string]any)
	if w.Code != 201 || m["email_sent"] != false || m["email_status"] != "failed" || c == nil || c["temporary_password"] == "" ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body)
	}
	var st string
	if err := e.d.GetContext(context.Background(), &st, `SELECT delivery_status FROM user_invitations`); err != nil || st != "failed" {
		t.Fatalf("status=%q err=%v", st, err)
	}
	var failed int
	for _, a := range e.auditSnapshot() {
		if a.Action == "user.invitation.email_failed" && a.Status == record.StatusError {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("audit email_failed=%d", failed)
	}
	// mailer tidak terkonfigurasi
	e2 := setupWireMail(t, false, nil)
	ac2 := e2.adminCookie("admin@tim.id")
	w = e2.do("POST", "/api/users/invite", inviteBody("n@tim.id"), ac2)
	if w.Code != 201 || jsonMap(t, w)["email_status"] != "not_configured" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// kredensial tidak terbaca lagi
	pw := jsonMap(t, w)["credentials"].(map[string]any)["temporary_password"].(string)
	id := jsonMap(t, w)["user"].(map[string]any)["id"].(string)
	for _, p := range []string{"/api/users/" + id, "/api/users"} {
		if g := e2.do("GET", p, "", ac2); strings.Contains(g.Body.String(), pw) {
			t.Fatalf("password terbaca di %s", p)
		}
	}
}

func TestT12_AC1_PasswordNeverLoggedOrAudited(t *testing.T) {
	fk := &mailertest.Fake{}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	var secrets []string
	w := e.do("POST", "/api/users/invite", inviteBody("ok@tim.id"), ac)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	fk.Err = errors.New("smtp mati")
	w = e.do("POST", "/api/users/invite", inviteBody("fb@tim.id"), ac)
	secrets = append(secrets, jsonMap(t, w)["credentials"].(map[string]any)["temporary_password"].(string))
	var hashes []string
	if err := e.d.SelectContext(context.Background(), &hashes, `SELECT password_hash FROM users`); err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, "$argon2id$")
	secrets = append(secrets, hashes...)
	// password email sukses ada di isi email (hanya di sana)
	ab, _ := json.Marshal(e.auditSnapshot())
	blob := e.logs.String() + string(ab)
	for _, s := range secrets {
		if s != "" && strings.Contains(blob, s) {
			t.Fatalf("rahasia bocor: %q", s)
		}
	}
	if bytes.Contains(ab, []byte("temporary_password")) {
		t.Fatal("kunci temporary_password di audit")
	}
}

func TestT12_InviterRateLimit(t *testing.T) {
	fk := &mailertest.Fake{}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	for i := 0; i < 20; i++ {
		if w := e.do("POST", "/api/users/invite", inviteBody("u"+string(rune('a'+i))+"@tim.id"), ac); w.Code != 201 {
			t.Fatalf("ke-%d: %d %s", i+1, w.Code, w.Body)
		}
	}
	w := e.do("POST", "/api/users/invite", inviteBody("lagi@tim.id"), ac)
	d := jsonMap(t, w)["error"].(map[string]any)
	det := d["details"].(map[string]any)
	if w.Code != 429 || d["code"] != "RATE_LIMITED" || det["scope"] != "invite" || det["retry_after_seconds"].(float64) <= 0 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestT12_BusDeliverFalseThenLoginOnlyAfterIssue(t *testing.T) {
	fk := &mailertest.Fake{Err: errors.New("smtp mati")}
	e := setupWireMail(t, false, fk)
	e.seedAdmin("admin@tim.id")
	var adminID string
	_ = e.d.GetContext(context.Background(), &adminID, `SELECT id::text FROM users WHERE email='admin@tim.id'`)
	call := func(deliver bool) inviteuserResp {
		r, err := bus.CallTyped[inviteuserResp](context.Background(), e.bus, constants.BUS_CORE_AUTH_INVITE_USER,
			inviteuserReq{Email: "bus@tim.id", InvitedBy: adminID, InvitedByLabel: "Admin", ProjectName: "Alpha", Deliver: deliver})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := call(false)
	if !r.Created || r.EmailStatus != "pending" || r.Credentials != nil || len(fk.Sent) != 0 {
		t.Fatalf("r=%+v", r)
	}
	if c, _ := e.login("bus@tim.id", "apa-saja-12345"); c != nil {
		t.Fatal("login harus gagal sebelum diterbitkan")
	}
	r = call(true)
	if r.Created || r.Credentials == nil {
		t.Fatalf("r=%+v", r)
	}
	c, w := e.login("bus@tim.id", r.Credentials.TemporaryPassword)
	var mc bool
	_ = e.d.GetContext(context.Background(), &mc, `SELECT must_change_password FROM users WHERE email='bus@tim.id'`)
	if c == nil || !mc {
		t.Fatalf("login: %d %s mc=%v", w.Code, w.Body, mc)
	}
}

func TestT12_AuditEachActionOneEntry(t *testing.T) {
	fk := &mailertest.Fake{}
	e := setupWireMail(t, false, fk)
	ac := e.adminCookie("admin@tim.id")
	before := e.auditCount()
	w := e.do("POST", "/api/users/invite", inviteBody("a@tim.id"), ac)
	d := e.auditSnapshot()[before:]
	if w.Code != 201 || len(d) != 1 || d[0].Action != "user.invited" || d[0].ActorLabel != "admin@tim.id" ||
		d[0].TraceID == "" {
		t.Fatalf("%d %+v", w.Code, d)
	}
	fk.Err = errors.New("mati")
	before = e.auditCount()
	e.do("POST", "/api/users/invite", inviteBody("b@tim.id"), ac)
	d = e.auditSnapshot()[before:]
	if len(d) != 2 || d[0].Action != "user.invited" || d[1].Action != "user.invitation.email_failed" {
		t.Fatalf("%+v", d)
	}
	before = e.auditCount()
	e.do("POST", "/api/users/invite", inviteBody("bukan-email"), ac)
	e.do("POST", "/api/users/invite", inviteBody("admin@tim.id"), ac)
	if e.auditCount() != before {
		t.Fatal("400/409 tidak boleh diaudit")
	}
}

func TestT12_AuditUnregisteredDoesNotFailRequest(t *testing.T) {
	e := setupWireMail(t, true, &mailertest.Fake{})
	ac := e.adminCookie("admin@tim.id")
	if w := e.do("POST", "/api/users/invite", inviteBody("x@tim.id"), ac); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

type (
	inviteuserReq  = inviteuserc.Request
	inviteuserResp = inviteuserc.Response
)
