//go:build integration

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"grimoire/internal/domain/core/internal/auth/login"
	"grimoire/internal/domain/core/internal/auth/resolvesession"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	httpx "grimoire/internal/infra/http"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/migrations"
)

const loginPW = "rahasia-panjang-1"

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

type loginEnv struct {
	t      *testing.T
	d      *db.DB
	h      http.Handler
	logs   *syncBuf
	now    *time.Time
	mu     sync.Mutex
	audits []record.Request
	hasher *verifyCounter
	cfg    app.Deps
}

type verifyCounter struct {
	base.Hasher
	mu sync.Mutex
	n  int
}

func (v *verifyCounter) Verify(enc, pw string) (bool, error) {
	v.mu.Lock()
	v.n++
	v.mu.Unlock()
	return v.Hasher.Verify(enc, pw)
}
func (v *verifyCounter) count() int { v.mu.Lock(); defer v.mu.Unlock(); return v.n }

type envOpts struct {
	ratePerMinute int
	noAudit       bool
	argonMem      uint32
	secure        bool
}

func setupLogin(t *testing.T, o envOpts) *loginEnv {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Migrate(context.Background(), d, migrations.FS, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := testConfig()
	cfg.CookieSecure = o.secure
	cfg.HTTPDefaultBodyBytes = 1 << 20
	cfg.HTTPDefaultTimeout = 30 * time.Second
	cfg.LoginRatePerMinute = 6000
	cfg.SessionAbsoluteTimeout = 168 * time.Hour
	cfg.SessionIdleTimeout = 12 * time.Hour
	if o.ratePerMinute > 0 {
		cfg.LoginRatePerMinute = o.ratePerMinute
	}
	if o.argonMem > 0 {
		cfg.Argon2MemoryKiB = o.argonMem
	}
	logs := &syncBuf{}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	b := bus.NewRegistry(log)
	e := &loginEnv{t: t, d: d, logs: logs}
	if !o.noAudit {
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
	_ = root
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	deps := app.Deps{Ctx: ctx, Config: cfg, Log: log, DB: d, Bus: b, API: api, MCP: mcp}
	registerCodes()
	shared, err := buildShared(cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	e.now = &now
	shared.Clock = func() time.Time { return *e.now }
	e.hasher = &verifyCounter{Hasher: shared.Hasher}
	shared.Hasher = e.hasher
	resolvesession.Init(deps, shared)
	login.Init(deps, shared)
	api.Get("/_whoami", func(w http.ResponseWriter, r *http.Request) {
		id, ok := reqctx.IdentityFrom(r.Context())
		if !ok {
			http.Error(w, "anon", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, id.Email)
	})
	e.h = root
	e.cfg = deps
	return e
}

func (e *loginEnv) seed(email string, active, mustChange bool) string {
	e.t.Helper()
	h := base.Argon2Hasher{Params: base.Argon2Params{MemoryKiB: e.cfg.Config.Argon2MemoryKiB, Iterations: 1, Parallelism: 1}}
	hash, err := h.Hash(loginPW)
	if err != nil {
		e.t.Fatal(err)
	}
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO users (email, name, password_hash, role, is_active, must_change_password)
		VALUES ($1,'Budi',$2,'developer',$3,$4) RETURNING id::text`, email, hash, active, mustChange); err != nil {
		e.t.Fatal(err)
	}
	return id
}

type resp struct {
	*httptest.ResponseRecorder
}

func (e *loginEnv) post(ip, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	r.RemoteAddr = ip + ":4000"
	r.Header.Set("X-Requested-With", "grimoire")
	r.Header.Set("User-Agent", "TestUA")
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *loginEnv) login(email, pw string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"email": email, "password": pw})
	return e.post("10.0.0.5", string(b), nil)
}

func (e *loginEnv) auditsFor(action string) []record.Request {
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

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("body %s: %v", w.Body, err)
	}
	return b.Error.Code
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == "grimoire_session" {
			return c
		}
	}
	return nil
}

func (e *loginEnv) countRows(q string, args ...any) int {
	var n int
	if err := e.d.GetContext(context.Background(), &n, q, args...); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestT01_AC2_LoginSetsHttpOnlyCookie(t *testing.T) {
	e := setupLogin(t, envOpts{secure: true})
	e.seed("budi@tim.id", true, false)
	w := e.login("Budi@Tim.ID", loginPW)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	c := sessionCookie(w)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 604800 || !c.Secure {
		t.Fatalf("%+v", c)
	}
	// cookie dapat dipakai pada route terproteksi (Session middleware -> resolve-session).
	r := httptest.NewRequest(http.MethodGet, "/api/_whoami", nil)
	r.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	ww := httptest.NewRecorder()
	e.h.ServeHTTP(ww, r)
	if ww.Code != 200 || ww.Body.String() != "budi@tim.id" {
		t.Fatalf("whoami %d %s", ww.Code, ww.Body)
	}
	// DB hanya menyimpan hash.
	var hash []byte
	if err := e.d.GetContext(context.Background(), &hash, `SELECT token_hash FROM sessions`); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(hash, []byte(c.Value)) || string(hash) == c.Value {
		t.Fatal("token plaintext tersimpan")
	}
	if !bytes.Equal(hash, base.HashToken([]byte(e.cfg.Config.SessionSecret), c.Value)) {
		t.Fatal("hash tidak cocok HMAC token")
	}
	var out login.LoginResponse
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.User.Email != "budi@tim.id" {
		t.Fatalf("%+v", out)
	}
}

func normalize(w *httptest.ResponseRecorder) (int, string, map[string][]string) {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	delete(m, "trace_id")
	b, _ := json.Marshal(m)
	h := map[string][]string{}
	for k, v := range w.Header() {
		switch strings.ToLower(k) {
		case "x-request-id", "date", "x-trace-id":
			continue
		}
		h[k] = v
	}
	return w.Code, string(b), h
}

func TestT01_AC2_FailureResponsesIdentical(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("aktif@tim.id", true, false)
	e.seed("off@tim.id", false, false)
	w1 := e.login("tidakada@tim.id", loginPW)
	w2 := e.login("aktif@tim.id", "salah-salah-1")
	w3 := e.login("off@tim.id", loginPW)
	c1, b1, h1 := normalize(w1)
	for i, w := range []*httptest.ResponseRecorder{w2, w3} {
		c, b, h := normalize(w)
		if c != c1 || b != b1 || fmt.Sprint(sortedHdr(h)) != fmt.Sprint(sortedHdr(h1)) {
			t.Fatalf("kasus %d berbeda:\n%d %s %v\n%d %s %v", i, c, b, h, c1, b1, h1)
		}
	}
	if c1 != 401 || errCode(t, w1) != "INVALID_CREDENTIALS" {
		t.Fatalf("%d %s", c1, b1)
	}
}

func sortedHdr(h map[string][]string) []string {
	var out []string
	for k, v := range h {
		out = append(out, k+"="+strings.Join(v, ","))
	}
	sort.Strings(out)
	return out
}

func TestT01_AC2_FailureTimingEquivalent(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("aktif@tim.id", true, false)
	before := e.hasher.count()
	e.login("tidakada@tim.id", loginPW)
	if e.hasher.count()-before != 1 {
		t.Fatal("email tak ada: Verify harus tepat 1x")
	}
	before = e.hasher.count()
	e.login("aktif@tim.id", "salah-salah-1")
	if e.hasher.count()-before != 1 {
		t.Fatal("password salah: Verify harus tepat 1x")
	}
}

func TestT01_AC2_FailureTimingStatistical(t *testing.T) {
	if testing.Short() {
		t.Skip("statistik timing dilewati pada -short")
	}
	e := setupLogin(t, envOpts{argonMem: 8192})
	e.seed("aktif@tim.id", true, false)
	median := func(email string, pw string) time.Duration {
		var ds []time.Duration
		for i := 0; i < 20; i++ {
			// reset throttle agar tidak terkunci
			_, _ = e.d.Exec(`DELETE FROM login_throttles`)
			s := time.Now()
			e.login(email, pw)
			ds = append(ds, time.Since(s))
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[len(ds)/2]
	}
	unknown, wrong := median("tidakada@tim.id", loginPW), median("aktif@tim.id", "salah-salah-1")
	ratio := float64(unknown) / float64(wrong)
	if ratio < 0.5 || ratio > 2.0 {
		t.Fatalf("rasio waktu %.2f (unknown=%v wrong=%v)", ratio, unknown, wrong)
	}
}

func TestT01_AC3_LockAfterFiveFailures(t *testing.T) {
	e := setupLogin(t, envOpts{})
	uid := e.seed("budi@tim.id", true, false)
	for i := 1; i <= 5; i++ {
		if w := e.login("budi@tim.id", "salah-salah-1"); w.Code != 401 {
			t.Fatalf("percobaan %d: %d", i, w.Code)
		}
	}
	w := e.login("budi@tim.id", loginPW)
	if w.Code != 429 || errCode(t, w) != "ACCOUNT_LOCKED" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var b struct {
		Error struct {
			Details struct {
				Secs int `json:"retry_after_seconds"`
			}
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if b.Error.Details.Secs <= 0 || w.Header().Get("Retry-After") != fmt.Sprint(b.Error.Details.Secs) {
		t.Fatalf("retry-after %q vs %d", w.Header().Get("Retry-After"), b.Error.Details.Secs)
	}
	if e.countRows(`SELECT count(*) FROM sessions WHERE user_id=$1`, uid) != 0 {
		t.Fatal("tidak boleh ada sesi saat terkunci")
	}
	*e.now = e.now.Add(15*time.Minute + time.Second)
	if w := e.login("budi@tim.id", loginPW); w.Code != 200 {
		t.Fatalf("setelah lock habis: %d %s", w.Code, w.Body)
	}
	if e.countRows(`SELECT count(*) FROM login_throttles`) != 0 {
		t.Fatal("throttle harus terhapus setelah sukses")
	}
}

func TestT01_AC3_LockAppliesToUnknownEmail(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	for i := 0; i < 5; i++ {
		if w := e.login("hantu@tim.id", loginPW); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	wu := e.login("hantu@tim.id", loginPW)
	for i := 0; i < 5; i++ {
		e.login("budi@tim.id", "salah-salah-1")
	}
	wk := e.login("budi@tim.id", loginPW)
	if wu.Code != 429 || wk.Code != 429 || errCode(t, wu) != errCode(t, wk) || wu.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %d", wu.Code, wk.Code)
	}
	cu, bu, _ := normalize(wu)
	ck, bk, _ := normalize(wk)
	if cu != ck || strings.Count(bu, `"`) != strings.Count(bk, `"`) {
		t.Fatalf("bentuk berbeda: %s vs %s", bu, bk)
	}
}

func TestT01_AC3_LockedAttemptDoesNotExtendLock(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	for i := 0; i < 5; i++ {
		e.login("budi@tim.id", "salah-salah-1")
	}
	q := func() (time.Time, int) {
		var u time.Time
		var n int
		if err := e.d.QueryRowx(`SELECT locked_until, failed_count FROM login_throttles WHERE email='budi@tim.id'`).Scan(&u, &n); err != nil {
			t.Fatal(err)
		}
		return u, n
	}
	u1, n1 := q()
	*e.now = e.now.Add(time.Minute)
	e.login("budi@tim.id", "salah-salah-1")
	e.login("budi@tim.id", loginPW)
	u2, n2 := q()
	if !u1.Equal(u2) || n1 != n2 {
		t.Fatalf("lock berubah: %v/%d -> %v/%d", u1, n1, u2, n2)
	}
}

func TestT01_AC3_SuccessResetsCounter(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	for i := 0; i < 4; i++ {
		e.login("budi@tim.id", "salah-salah-1")
	}
	if w := e.login("budi@tim.id", loginPW); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 4; i++ {
		if w := e.login("budi@tim.id", "salah-salah-1"); w.Code != 401 {
			t.Fatalf("percobaan %d: %d", i, w.Code)
		}
	}
	if w := e.login("budi@tim.id", loginPW); w.Code != 200 {
		t.Fatalf("belum boleh terkunci: %d", w.Code)
	}
}

func TestLogin_RateLimitPerIP(t *testing.T) {
	e := setupLogin(t, envOpts{ratePerMinute: 4}) // rate 4/60 per detik, burst 1
	e.seed("budi@tim.id", true, false)
	body := `{"email":"budi@tim.id","password":"salah-salah-1"}`
	if w := e.post("10.1.1.1", body, nil); w.Code != 401 {
		t.Fatalf("pertama: %d", w.Code)
	}
	w := e.post("10.1.1.1", body, nil)
	if w.Code != 429 || errCode(t, w) != "RATE_LIMITED" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := e.post("10.2.2.2", body, nil); w.Code != 401 {
		t.Fatalf("IP lain tak boleh terpengaruh: %d", w.Code)
	}
}

func TestLogin_RequiresCSRFHeader(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	w := e.post("10.0.0.5", fmt.Sprintf(`{"email":"budi@tim.id","password":%q}`, loginPW), map[string]string{"X-Requested-With": ""})
	if w.Code != 403 || errCode(t, w) != "CSRF_REQUIRED" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestLogin_MustChangePasswordUserCanLogin(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, true)
	w := e.login("budi@tim.id", loginPW)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"must_change_password":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestLogin_StaleCookieIgnored(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	body := fmt.Sprintf(`{"email":"budi@tim.id","password":%q}`, loginPW)
	w := e.post("10.0.0.5", body, map[string]string{"Cookie": "grimoire_session=basi-basi-basi"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	cs := w.Result().Cookies()
	last := cs[len(cs)-1]
	if last.Name != "grimoire_session" || last.Value == "basi-basi-basi" || last.MaxAge != 604800 {
		t.Fatalf("cookie akhir harus sesi baru: %+v", cs)
	}
}

func TestLogin_AC8_NoSecretsInLogsOrAudit(t *testing.T) {
	e := setupLogin(t, envOpts{})
	e.seed("budi@tim.id", true, false)
	okW := e.login("budi@tim.id", loginPW)
	e.login("budi@tim.id", "salah-rahasia-9")
	token := sessionCookie(okW).Value
	var hash string
	_ = e.d.GetContext(context.Background(), &hash, `SELECT password_hash FROM users`)
	ab, _ := json.Marshal(e.audits)
	all := e.logs.String() + string(ab)
	for _, bad := range []string{loginPW, "salah-rahasia-9", token, hash, "$argon2id$"} {
		if strings.Contains(all, bad) {
			t.Fatalf("rahasia bocor: %q", bad)
		}
	}
}

func TestLogin_AC9_AuditExactlyOnce(t *testing.T) {
	e := setupLogin(t, envOpts{})
	uid := e.seed("budi@tim.id", true, false)
	e.login("budi@tim.id", loginPW)
	s := e.auditsFor("auth.login.success")
	if len(s) != 1 || s[0].ActorType != "user" || s[0].ActorID != uid || s[0].Status != "ok" || s[0].IP != "10.0.0.5" ||
		s[0].UserAgent != "TestUA" || s[0].TraceID == "" || len(e.auditsFor("auth.login.failed")) != 0 {
		t.Fatalf("%+v", e.audits)
	}
	e.login("budi@tim.id", "salah-salah-1")
	f := e.auditsFor("auth.login.failed")
	if len(f) != 1 || f[0].Status != "denied" || f[0].Args["reason"] != "invalid_credentials" || len(e.auditsFor("auth.login.success")) != 1 {
		t.Fatalf("%+v", e.audits)
	}
}

func TestAudit_UnregisteredDoesNotFailRequest(t *testing.T) {
	e := setupLogin(t, envOpts{noAudit: true})
	e.seed("budi@tim.id", true, false)
	if w := e.login("budi@tim.id", loginPW); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	l := e.logs.String()
	if !strings.Contains(l, "audit record gagal") || !strings.Contains(l, `"trace_id"`) {
		t.Fatalf("log: %s", l)
	}
}

func TestLogin_RouteRegisteredAtAPIPath(t *testing.T) {
	e := setupLogin(t, envOpts{})
	found := false
	_ = chi.Walk(e.h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost && route == "/api/auth/login" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("POST /api/auth/login tidak terdaftar")
	}
}
