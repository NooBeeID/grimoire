//go:build integration

package apikey

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/apikey/touch"
	"grimoire/lib/contracts/project/apikey/verify"
	"grimoire/lib/contracts/project/management/checkaccess"
	"grimoire/lib/contracts/project/management/contentcount"
	"grimoire/lib/contracts/project/management/getproject"
)

const (
	uidA = "0192f5a0-aaaa-7aaa-8bbb-00000000000a"
	uidB = "0192f5a0-aaaa-7aaa-8bbb-00000000000b"
	uidC = "0192f5a0-aaaa-7aaa-8bbb-00000000000c"
	uidS = "0192f5a0-aaaa-7aaa-8bbb-00000000005a"
)

var userRoles = map[string]string{uidA: "developer", uidB: "developer", uidC: "developer", uidS: "super_admin"}

type env struct {
	t        *testing.T
	d        *db.DB
	bus      *bus.Registry
	r        chi.Router
	logBuf   *syncBuf
	p1, p2   string
	mu       sync.Mutex
	acl      map[string]checkaccess.Level // userID|projectID
	names    map[string]string            // projectID -> nama
	audits   []record.Request
	auditErr error
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func setup(t *testing.T, limit int) *env {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	buf := &syncBuf{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e := &env{t: t, d: d, logBuf: buf, acl: map[string]checkaccess.Level{}, names: map[string]string{}}
	e.bus = bus.NewRegistry(log)

	e.bus.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(_ context.Context, req any) (any, error) {
		in := req.(checkaccess.Request)
		e.mu.Lock()
		defer e.mu.Unlock()
		if _, ok := e.names[in.ProjectID]; !ok {
			return checkaccess.Response{}, nil
		}
		if in.UserRole == "super_admin" {
			return checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelAdmin}, nil
		}
		lv, ok := e.acl[in.UserID+"|"+in.ProjectID]
		if !ok {
			lv = checkaccess.LevelNone
		}
		return checkaccess.Response{ProjectExists: true, Level: lv}, nil
	})
	e.bus.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(_ context.Context, req any) (any, error) {
		in := req.(getproject.Request)
		e.mu.Lock()
		defer e.mu.Unlock()
		n, ok := e.names[in.ProjectID]
		return getproject.Response{Found: ok, ID: in.ProjectID, Name: n}, nil
	})
	e.bus.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.auditErr != nil {
			return nil, e.auditErr
		}
		e.audits = append(e.audits, req.(record.Request))
		return record.Response{Recorded: true}, nil
	})

	root := chi.NewRouter()
	root.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := r.Header.Get("X-Test-User"); u != "" {
				r = r.WithContext(reqctx.WithIdentity(r.Context(), reqctx.Identity{UserID: u, Role: userRoles[u], Email: u[len(u)-2:] + "@tim.id"}))
			}
			next.ServeHTTP(w, r)
		})
	})
	api := chi.NewRouter()
	cfg := config.Config{APIKeyMaxActivePerProject: limit, PublicURL: "https://g.tim.id"}
	Register(app.Deps{Ctx: context.Background(), Config: cfg, Log: log, DB: d, Bus: e.bus, API: api})
	root.Mount("/api", api)
	e.r = root

	e.p1 = e.mkProject("P1")
	e.p2 = e.mkProject("P2")
	e.acl[uidA+"|"+e.p1] = checkaccess.LevelOwner
	e.acl[uidB+"|"+e.p1] = checkaccess.LevelMember
	e.acl[uidA+"|"+e.p2] = checkaccess.LevelOwner
	return e
}

func (e *env) mkProject(name string) string {
	e.t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		e.t.Fatal(err)
	}
	e.mu.Lock()
	e.names[id] = name
	e.mu.Unlock()
	return id
}

func (e *env) do(method, path, user, body string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	e.r.ServeHTTP(rec, req)
	return rec
}

type created struct {
	Key struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Prefix string `json:"prefix"`
	} `json:"key"`
	RawKey string  `json:"raw_key"`
	MCPURL *string `json:"mcp_url"`
}

func (e *env) create(user, pid, name string) (*httptest.ResponseRecorder, created) {
	rec := e.do(http.MethodPost, "/api/projects/"+pid+"/api-keys", user, `{"name":"`+name+`"}`)
	var c created
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
			e.t.Fatalf("decode create: %v %s", err, rec.Body)
		}
	}
	return rec, c
}

func (e *env) mustCreate(user, pid, name string) created {
	e.t.Helper()
	rec, c := e.create(user, pid, name)
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("create %q: %d %s", name, rec.Code, rec.Body)
	}
	return c
}

func (e *env) revoke(user, pid, kid string) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, "/api/projects/"+pid+"/api-keys/"+kid+"/revoke", user, "")
}

func (e *env) list(user, pid string) *httptest.ResponseRecorder {
	return e.do(http.MethodGet, "/api/projects/"+pid+"/api-keys", user, "")
}

func (e *env) verify(raw string) verify.Response {
	e.t.Helper()
	res, err := bus.CallTyped[verify.Response](context.Background(), e.bus, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})
	if err != nil {
		e.t.Fatalf("verify: %v", err)
	}
	return res
}

func (e *env) auditActions(action string) []record.Request {
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

func (e *env) activeCount(pid string) int {
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM api_keys WHERE project_id=$1 AND revoked_at IS NULL`, pid); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func secretOf(raw string) string {
	parts := strings.SplitN(raw, "_", 3)
	return parts[2]
}

func bodyHas(rec *httptest.ResponseRecorder, s string) bool {
	return strings.Contains(rec.Body.String(), s)
}

func TestT04_AC1_MemberCreatesKey_NonMember404_SuperAdminOK(t *testing.T) {
	e := setup(t, 20)
	if rec, c := e.create(uidB, e.p1, "b-key"); rec.Code != 201 || c.MCPURL == nil || *c.MCPURL != "https://g.tim.id/mcp" {
		t.Fatalf("member: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := e.create(uidC, e.p1, "c-key"); rec.Code != 404 || !bodyHas(rec, "project tidak ditemukan") {
		t.Fatalf("non-member: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := e.create(uidS, e.p1, "s-key"); rec.Code != 201 {
		t.Fatalf("super_admin: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := e.create(uidA, "bukan-uuid", "x"); rec.Code != 404 {
		t.Fatalf("non-uuid: %d", rec.Code)
	}
}

func TestT04_AC2_PlaintextOnlyInCreateResponse(t *testing.T) {
	e := setup(t, 20)
	rec, c := e.create(uidA, e.p1, "ci")
	if rec.Code != 201 || len(c.RawKey) != 55 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create: %d len=%d cc=%q", rec.Code, len(c.RawKey), rec.Header().Get("Cache-Control"))
	}
	secret := secretOf(c.RawKey)
	lr := e.list(uidA, e.p1)
	if lr.Code != 200 || bodyHas(lr, "raw_key") || bodyHas(lr, secret) || bodyHas(lr, c.RawKey) {
		t.Fatalf("list leaks: %d %s", lr.Code, lr.Body)
	}
	var rows []string
	if err := e.d.SelectContext(context.Background(), &rows, `SELECT row_to_json(k)::text FROM api_keys k`); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if strings.Contains(r, c.RawKey) || strings.Contains(r, secret) {
			t.Fatalf("row leaks secret: %s", r)
		}
	}
	var hash []byte
	if err := e.d.GetContext(context.Background(), &hash, `SELECT secret_hash FROM api_keys WHERE id=$1`, c.Key.ID); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(secret))
	if !bytes.Equal(hash, sum[:]) {
		t.Fatalf("secret_hash %s != sha256(secret) %s", hex.EncodeToString(hash), hex.EncodeToString(sum[:]))
	}
}

func TestT04_AC3_VerifyImmediatelyAfterCreateAndRevoke(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	v := e.verify(c.RawKey)
	if !v.Found || v.KeyID != c.Key.ID || v.KeyName != "ci" || v.Prefix != c.Key.Prefix || v.ProjectID != e.p1 {
		t.Fatalf("verify after create: %+v", v)
	}
	if rec := e.revoke(uidB, e.p1, c.Key.ID); rec.Code != 200 {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
	}
	v = e.verify(c.RawKey)
	if v.Found || v.Reason != verify.ReasonRevoked || v.Prefix != c.Key.Prefix {
		t.Fatalf("verify after revoke: %+v", v)
	}
}

func TestT04_AC4_DuplicateActiveNameRejected_ReusableAfterRevoke(t *testing.T) {
	e := setup(t, 20)
	first := e.mustCreate(uidA, e.p1, "ci")
	rec, _ := e.create(uidA, e.p1, "ci")
	if rec.Code != 409 || !bodyHas(rec, "NAME_TAKEN") {
		t.Fatalf("dup: %d %s", rec.Code, rec.Body)
	}
	if r := e.revoke(uidA, e.p1, first.Key.ID); r.Code != 200 {
		t.Fatalf("revoke: %d", r.Code)
	}
	e.mustCreate(uidA, e.p1, "ci")
	lr := e.list(uidA, e.p1)
	var res struct {
		Items []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(lr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	var active, revoked int
	for _, it := range res.Items {
		if it.Name != "ci" {
			t.Fatalf("name: %+v", it)
		}
		if it.Status == "active" {
			active++
		} else {
			revoked++
		}
	}
	if active != 1 || revoked != 1 {
		t.Fatalf("items: %+v", res.Items)
	}
}

func TestT04_AC5_ActiveLimitEnforced(t *testing.T) {
	e := setup(t, 3)
	var first created
	for i, n := range []string{"k1", "k2", "k3"} {
		c := e.mustCreate(uidA, e.p1, n)
		if i == 0 {
			first = c
		}
	}
	rec, _ := e.create(uidA, e.p1, "k4")
	if rec.Code != 409 || !bodyHas(rec, "KEY_LIMIT_REACHED") {
		t.Fatalf("limit: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil && body.Error.Details != nil {
		if l, _ := body.Error.Details["limit"].(float64); l != 3 {
			t.Fatalf("limit detail: %v", body.Error.Details)
		}
	} else if !bodyHas(rec, `"limit":3`) {
		t.Fatalf("limit detail missing: %s", rec.Body)
	}
	if r := e.revoke(uidA, e.p1, first.Key.ID); r.Code != 200 {
		t.Fatalf("revoke: %d", r.Code)
	}
	e.mustCreate(uidA, e.p1, "k5")

	// Dua create serentak saat sisa 1 slot.
	for i := 0; i < 10; i++ {
		pid := e.mkProject("race")
		e.mu.Lock()
		e.acl[uidA+"|"+pid] = checkaccess.LevelOwner
		e.mu.Unlock()
		e.mustCreate(uidA, pid, "key-a")
		e.mustCreate(uidA, pid, "key-b")
		codes := make([]int, 2)
		var wg sync.WaitGroup
		for j := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := e.create(uidA, pid, "race-"+string(rune('0'+j)))
				codes[j] = r.Code
			}()
		}
		wg.Wait()
		sort.Ints(codes)
		if codes[0] != 201 || codes[1] != 409 {
			t.Fatalf("iter %d: codes=%v", i, codes)
		}
		if n := e.activeCount(pid); n != 3 {
			t.Fatalf("iter %d: active=%d", i, n)
		}
	}
}

func TestT04_AC6_CreatorRemovalDoesNotKillKey(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	e.mu.Lock()
	e.acl[uidA+"|"+e.p1] = checkaccess.LevelNone
	e.mu.Unlock()
	if v := e.verify(c.RawKey); !v.Found {
		t.Fatalf("verify: %+v", v)
	}
	if rec := e.list(uidA, e.p1); rec.Code != 404 {
		t.Fatalf("list after removal: %d", rec.Code)
	}
}

func TestT04_AC7_TouchWritesOncePerMinute(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	ctx := context.Background()
	lastUsed := func() (*time.Time, bool) {
		var ts *time.Time
		if err := e.d.GetContext(ctx, &ts, `SELECT last_used_at FROM api_keys WHERE id=$1`, c.Key.ID); err != nil {
			t.Fatal(err)
		}
		return ts, ts != nil
	}
	if _, ok := lastUsed(); ok {
		t.Fatal("last_used_at harus kosong sebelum touch")
	}
	touchOnce := func() {
		// Call (sinkron) ke handler yang sama dengan Publish agar hasil bisa dibaca deterministik.
		if _, err := e.bus.Call(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: c.Key.ID}); err != nil {
			t.Fatal(err)
		}
	}
	touchOnce()
	first, ok := lastUsed()
	if !ok {
		t.Fatal("touch pertama harus menulis last_used_at")
	}
	for i := 0; i < 4; i++ {
		touchOnce()
	}
	if again, _ := lastUsed(); !again.Equal(*first) {
		t.Fatalf("touch dalam menit yang sama mengubah nilai: %v -> %v", first, again)
	}
	// Mundurkan last_used_at 61 detik -> touch berikutnya menulis lagi.
	back := first.Add(-61 * time.Second)
	if _, err := e.d.ExecContext(ctx, `UPDATE api_keys SET last_used_at=$2 WHERE id=$1`, c.Key.ID, back); err != nil {
		t.Fatal(err)
	}
	touchOnce()
	if after, _ := lastUsed(); !after.After(back) {
		t.Fatalf("touch setelah >60 detik tidak menulis: %v", after)
	}
	lr := e.list(uidA, e.p1)
	if !bodyHas(lr, `"last_used_at":"20`) {
		t.Fatalf("list tanpa last_used_at: %s", lr.Body)
	}
}

func TestT04_AC9_AuditCreatedAndRevoked(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	if r := e.revoke(uidB, e.p1, c.Key.ID); r.Code != 200 {
		t.Fatalf("revoke: %d", r.Code)
	}
	cr, rv := e.auditActions(record.ActionAPIKeyCreated), e.auditActions(record.ActionAPIKeyRevoked)
	if len(cr) != 1 || len(rv) != 1 {
		t.Fatalf("created=%d revoked=%d", len(cr), len(rv))
	}
	for _, a := range append(cr, rv...) {
		if a.ProjectLabel != "P1" || a.ProjectID != e.p1 || a.Target != c.Key.ID || a.TargetType != "api_key" ||
			a.Args["name"] != "ci" || a.Args["prefix"] != c.Key.Prefix || a.Status != record.StatusOK {
			t.Fatalf("audit: %+v", a)
		}
		raw, _ := json.Marshal(a)
		if strings.Contains(string(raw), c.RawKey) || strings.Contains(string(raw), secretOf(c.RawKey)) {
			t.Fatalf("audit leaks secret: %s", raw)
		}
	}
	if cr[0].ActorID != uidA || rv[0].ActorID != uidB {
		t.Fatalf("actors: %s %s", cr[0].ActorID, rv[0].ActorID)
	}

	// Gagal -> tanpa audit baru.
	e.create(uidA, e.p1, "x")      // 400 nama terlalu pendek
	e.create(uidA, e.p1, "")       // 400
	e.create(uidC, e.p1, "zzz")    // 404
	e.revoke(uidA, e.p1, c.Key.ID) // 409 sudah dicabut
	if len(e.auditActions(record.ActionAPIKeyCreated)) != 1 || len(e.auditActions(record.ActionAPIKeyRevoked)) != 1 {
		t.Fatal("kegagalan tidak boleh mengaudit")
	}

	// Audit handler gagal -> respons tetap sukses.
	e.mu.Lock()
	e.auditErr = errors.New("audit down")
	e.mu.Unlock()
	c2, _ := e.create(uidA, e.p1, "ci2")
	if c2.Code != 201 {
		t.Fatalf("create dengan audit gagal: %d", c2.Code)
	}
	var cc created
	_ = json.Unmarshal(c2.Body.Bytes(), &cc)
	if r := e.revoke(uidA, e.p1, cc.Key.ID); r.Code != 200 {
		t.Fatalf("revoke dengan audit gagal: %d", r.Code)
	}
}

func TestT04_Security_ProjectIsolation(t *testing.T) {
	e := setup(t, 20)
	k2 := e.mustCreate(uidA, e.p2, "p2-key")
	e.mustCreate(uidA, e.p1, "p1-key")

	lr := e.list(uidA, e.p1)
	if lr.Code != 200 || bodyHas(lr, "p2-key") || bodyHas(lr, k2.Key.ID) {
		t.Fatalf("list P1 memuat key P2: %s", lr.Body)
	}
	rec := e.revoke(uidA, e.p1, k2.Key.ID)
	if rec.Code != 404 || !bodyHas(rec, "KEY_NOT_FOUND") {
		t.Fatalf("revoke lintas project: %d %s", rec.Code, rec.Body)
	}
	if rec := e.revoke(uidA, e.p1, "bukan-uuid"); rec.Code != 404 || !bodyHas(rec, "KEY_NOT_FOUND") {
		t.Fatalf("keyID non-uuid: %d %s", rec.Code, rec.Body)
	}
	var revokedAt *time.Time
	if err := e.d.GetContext(context.Background(), &revokedAt, `SELECT revoked_at FROM api_keys WHERE id=$1`, k2.Key.ID); err != nil || revokedAt != nil {
		t.Fatalf("key P2 harus tetap aktif: %v %v", revokedAt, err)
	}
	// C (non-anggota) -> 404 seragam di ketiga route.
	r1, _ := e.create(uidC, e.p1, "xx")
	r2 := e.list(uidC, e.p1)
	r3 := e.revoke(uidC, e.p1, k2.Key.ID)
	for i, r := range []*httptest.ResponseRecorder{r1, r2, r3} {
		if r.Code != 404 || !bodyHas(r, "project tidak ditemukan") {
			t.Fatalf("route %d: %d %s", i, r.Code, r.Body)
		}
	}
}

func TestT04_Security_RevokedKeyRejected(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	if !e.verify(c.RawKey).Found {
		t.Fatal("harus ditemukan sebelum revoke")
	}
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := e.revoke(uidA, e.p1, c.Key.ID)
			codes[i] = rec.Code
			if rec.Code == 409 && !bodyHas(rec, "ALREADY_REVOKED") {
				t.Errorf("409 tanpa ALREADY_REVOKED: %s", rec.Body)
			}
		}()
	}
	wg.Wait()
	sort.Ints(codes)
	if codes[0] != 200 || codes[1] != 409 {
		t.Fatalf("codes=%v", codes)
	}
	if v := e.verify(c.RawKey); v.Found || v.Reason != verify.ReasonRevoked {
		t.Fatalf("verify: %+v", v)
	}
	if n := len(e.auditActions(record.ActionAPIKeyRevoked)); n != 1 {
		t.Fatalf("audit revoked=%d", n)
	}
}

func TestT04_Security_NoSecretInLogs(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	e.verify(c.RawKey)
	e.verify(c.RawKey[:len(c.RawKey)-1] + "x")
	e.revoke(uidA, e.p1, c.Key.ID)
	e.verify(c.RawKey)
	out := e.logBuf.String()
	if strings.Contains(out, c.RawKey) || strings.Contains(out, secretOf(c.RawKey)) {
		t.Fatalf("log memuat secret:\n%s", out)
	}
}

func TestT04_CountKeys_OnlyActive(t *testing.T) {
	e := setup(t, 20)
	e.mustCreate(uidA, e.p1, "key-a")
	gone := e.mustCreate(uidA, e.p1, "key-b")
	e.mustCreate(uidA, e.p1, "key-c")
	e.mustCreate(uidA, e.p2, "key-d")
	if r := e.revoke(uidA, e.p1, gone.Key.ID); r.Code != 200 {
		t.Fatalf("revoke: %d", r.Code)
	}
	res, err := bus.CallTyped[contentcount.Response](context.Background(), e.bus, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
		contentcount.Request{ProjectIDs: []string{e.p1, e.p2, "bukan-uuid"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts[e.p1] != 2 || res.Counts[e.p2] != 1 {
		t.Fatalf("counts=%v", res.Counts)
	}
	if _, ok := res.Counts["bukan-uuid"]; ok {
		t.Fatalf("id non-uuid harus diabaikan: %v", res.Counts)
	}
	ids := make([]string, contentcount.MaxProjectIDs+1)
	for i := range ids {
		ids[i] = e.p1
	}
	_, err = bus.CallTyped[contentcount.Response](context.Background(), e.bus, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
		contentcount.Request{ProjectIDs: ids})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("101 id: %v", err)
	}
}

func TestT04_CascadeOnProjectDelete(t *testing.T) {
	e := setup(t, 20)
	c := e.mustCreate(uidA, e.p1, "ci")
	if _, err := e.d.ExecContext(context.Background(), `DELETE FROM projects WHERE id=$1`, e.p1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM api_keys WHERE project_id=$1`, e.p1); err != nil || n != 0 {
		t.Fatalf("keys tersisa: %d %v", n, err)
	}
	if v := e.verify(c.RawKey); v.Found || v.Reason != verify.ReasonUnknown {
		t.Fatalf("verify: %+v", v)
	}
}

func TestRoutesRegistered(t *testing.T) {
	e := setup(t, 20)
	var got []string
	err := chi.Walk(e.r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{
		"GET /api/projects/{projectID}/api-keys",
		"POST /api/projects/{projectID}/api-keys",
		"POST /api/projects/{projectID}/api-keys/{keyID}/revoke",
	}
	if len(got) != len(want) {
		t.Fatalf("routes=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("routes=%v want=%v", got, want)
		}
	}

	ctx := context.Background()
	if _, err := e.bus.Call(ctx, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: "x"}); errors.Is(err, bus.ErrNotRegistered) || err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := e.bus.Call(ctx, constants.BUS_PROJECT_APIKEY_COUNT_KEYS, contentcount.Request{ProjectIDs: []string{e.p1}}); err != nil {
		t.Fatalf("count-keys: %v", err)
	}
	if err := e.bus.Publish(ctx, constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: "bukan-uuid"}); errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("touch: %v", err)
	}
}
