//go:build integration

package deleteproject

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/contentcount"
)

type env struct {
	d      *db.DB
	b      *bus.Registry
	h      http.Handler
	router chi.Router
	mu     sync.Mutex
	audits []record.Request
}

func migrated(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := d.ExecContext(context.Background(), `CREATE TABLE project_child (id uuid primary key default gen_random_uuid(), project_id uuid not null references projects(id) on delete cascade)`); err != nil {
		t.Fatal(err)
	}
	return d
}

func setup(t *testing.T) *env {
	t.Helper()
	d := migrated(t)
	e := &env{d: d, b: bus.NewRegistry(nil)}
	e.b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.audits = append(e.audits, req.(record.Request))
		return record.Response{Recorded: true}, nil
	})
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if uid := req.Header.Get("X-Test-Uid"); uid != "" {
				req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: uid, Role: req.Header.Get("X-Test-Role"), Email: uid + "@x.id"}))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(middleware.CSRF())
	api := chi.NewRouter()
	Init(app.Deps{Ctx: context.Background(), DB: d, Bus: e.b, API: api})
	r.Mount("/api", api)
	e.router, e.h = r, r
	return e
}

func (e *env) auditList() []record.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]record.Request(nil), e.audits...)
}

func (e *env) project(t *testing.T, name string, members map[string]string) string {
	t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	for u, role := range members {
		if _, err := e.d.ExecContext(context.Background(), `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, id, u, role); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := e.d.ExecContext(context.Background(), `INSERT INTO project_child (project_id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (e *env) count(t *testing.T, table, pid string) int {
	t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM `+table+` WHERE project_id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) exists(t *testing.T, pid string) bool {
	t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects WHERE id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func (e *env) del(uid, role, pid, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+url.PathEscape(pid), strings.NewReader(body))
	req.Header.Set("X-Requested-With", "grimoire")
	req.Header.Set("User-Agent", "UA/1.0")
	req.RemoteAddr = "10.0.0.5:1234"
	if uid != "" {
		req.Header.Set("X-Test-Uid", uid)
		req.Header.Set("X-Test-Role", role)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

type errBody struct {
	Error struct {
		Code, Message string
		Details       map[string]any
	} `json:"error"`
}

func parse(t *testing.T, w *httptest.ResponseRecorder) errBody {
	t.Helper()
	var b errBody
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b
}

func nameBody(n string) string {
	b, _ := json.Marshal(map[string]string{"confirm_name": n})
	return string(b)
}

func TestT03_AC6_DeleteRequiresConfirmationAndCascades(t *testing.T) {
	e := setup(t)
	owner, other := uuid.NewString(), uuid.NewString()
	pid := e.project(t, "Payments v2", map[string]string{owner: "owner", other: "member"})

	w := e.del(owner, "developer", pid, nameBody("salah"))
	if b := parse(t, w); w.Code != 400 || b.Error.Details["reason"] != "CONFIRMATION_MISMATCH" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := e.del(owner, "developer", pid, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if !e.exists(t, pid) || e.count(t, "project_members", pid) != 2 || e.count(t, "project_child", pid) != 2 || len(e.auditList()) != 0 {
		t.Fatal("data berubah setelah penolakan")
	}

	if w := e.del(owner, "developer", pid, nameBody("Payments v2")); w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if e.exists(t, pid) || e.count(t, "project_members", pid) != 0 || e.count(t, "project_child", pid) != 0 {
		t.Fatal("cascade gagal")
	}
	as := e.auditList()
	if len(as) != 1 {
		t.Fatalf("audit: %d", len(as))
	}
	a := as[0]
	if a.Action != record.ActionProjectDeleted || a.ProjectID != pid || a.ProjectLabel != "Payments v2" ||
		a.ActorType != record.ActorUser || a.ActorID != owner || a.UserAgent != "UA/1.0" || a.IP == "" ||
		a.Args["name"] != "Payments v2" || a.Args["member_count"] != 2 {
		t.Fatalf("%+v", a)
	}
}

func nonCascadeFKs(t *testing.T, d *db.DB) []string {
	t.Helper()
	var rows []struct {
		Rel  string `db:"rel"`
		Name string `db:"name"`
		Type string `db:"typ"`
	}
	if err := d.SelectContext(context.Background(), &rows, `SELECT conrelid::regclass::text AS rel, conname AS name, confdeltype::text AS typ FROM pg_constraint WHERE contype = 'f' AND confrelid = 'projects'::regclass`); err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, r := range rows {
		if r.Type != "c" {
			bad = append(bad, r.Rel+" ("+r.Name+")")
		}
	}
	return bad
}

func TestT03_AC6_AllProjectFKsCascade(t *testing.T) {
	d := migrated(t)
	if bad := nonCascadeFKs(t, d); len(bad) > 0 {
		t.Fatalf("FK ke projects(id) tanpa ON DELETE CASCADE: %v", bad)
	}
}

func TestT03_AC6_FKCheckDetectsViolation(t *testing.T) {
	d := migrated(t)
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `CREATE TABLE bad_child (project_id uuid references projects(id))`); err != nil {
		t.Fatal(err)
	}
	bad := nonCascadeFKs(t, d)
	if len(bad) != 1 || !strings.HasPrefix(bad[0], "bad_child") {
		t.Fatalf("%v", bad)
	}
	var pid string
	_ = d.GetContext(ctx, &pid, `INSERT INTO projects (name) VALUES ('Bad') RETURNING id::text`)
	_, _ = d.ExecContext(ctx, `INSERT INTO bad_child VALUES ($1)`, pid)
	if _, err := d.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, pid); err == nil {
		t.Fatal("harus error FK")
	}
}

func TestT03_AC6_OwnerMemberAdmin(t *testing.T) {
	e := setup(t)
	owner, member, sa := uuid.NewString(), uuid.NewString(), uuid.NewString()
	p1 := e.project(t, "Proj One", map[string]string{owner: "owner", member: "member"})
	if w := e.del(member, "developer", p1, nameBody("Proj One")); w.Code != 403 || !e.exists(t, p1) || len(e.auditList()) != 0 {
		t.Fatalf("member: %d", w.Code)
	}
	if w := e.del(owner, "developer", p1, nameBody("Proj One")); w.Code != 204 {
		t.Fatalf("owner: %d", w.Code)
	}
	p2 := e.project(t, "Proj Two", map[string]string{owner: "owner"})
	if w := e.del(sa, "super_admin", p2, nameBody("Proj Two")); w.Code != 204 {
		t.Fatalf("admin: %d", w.Code)
	}
	as := e.auditList()
	if len(as) != 2 || as[1].ActorID != sa || as[1].Args["member_count"] != 1 {
		t.Fatalf("%+v", as)
	}
}

func TestDelete_NonMemberAndUnknownIsUniform404(t *testing.T) {
	e := setup(t)
	owner, dev := uuid.NewString(), uuid.NewString()
	pid := e.project(t, "Secret", map[string]string{owner: "owner"})
	var first string
	for _, id := range []string{pid, uuid.NewString(), "bukan-uuid", "' OR 1=1", strings.Repeat("a", 500)} {
		w := e.del(dev, "developer", id, nameBody("Secret"))
		b := parse(t, w)
		if w.Code != 404 || b.Error.Code != "NOT_FOUND" || b.Error.Message != "project tidak ditemukan" || b.Error.Details != nil {
			t.Fatalf("%q: %d %s", id, w.Code, w.Body.String())
		}
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		delete(m, "trace_id")
		out, _ := json.Marshal(m)
		if first == "" {
			first = string(out)
		} else if string(out) != first {
			t.Fatalf("body berbeda: %s vs %s", out, first)
		}
	}
	if !e.exists(t, pid) || len(e.auditList()) != 0 {
		t.Fatal("efek samping")
	}
}

func TestDelete_RequiresAuthAndCSRF(t *testing.T) {
	e := setup(t)
	owner := uuid.NewString()
	pid := e.project(t, "Proj", map[string]string{owner: "owner"})
	if w := e.del("", "", pid, nameBody("Proj")); w.Code != 401 {
		t.Fatal(w.Code)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+pid, strings.NewReader(nameBody("Proj")))
	req.Header.Set("X-Test-Uid", owner)
	req.Header.Set("X-Test-Role", "developer")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 403 || parse(t, w).Error.Code != "CSRF_REQUIRED" || !e.exists(t, pid) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestDelete_OtherProjectsUntouched(t *testing.T) {
	e := setup(t)
	o1, o2 := uuid.NewString(), uuid.NewString()
	a := e.project(t, "Alpha", map[string]string{o1: "owner"})
	b := e.project(t, "Bravo", map[string]string{o2: "owner", o1: "member"})
	if w := e.del(o1, "developer", a, nameBody("Alpha")); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if !e.exists(t, b) || e.count(t, "project_members", b) != 2 || e.count(t, "project_child", b) != 2 {
		t.Fatal("project lain berubah")
	}
	// isolasi: o1 bukan anggota Bravo? (anggota member) -> 403; user lain -> 404
	if w := e.del(uuid.NewString(), "developer", b, nameBody("Bravo")); w.Code != 404 || !e.exists(t, b) {
		t.Fatal(w.Code)
	}
}

func TestDelete_CountsFromProviders(t *testing.T) {
	e := setup(t)
	owner := uuid.NewString()
	pid := e.project(t, "Counted", map[string]string{owner: "owner"})
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, func(_ context.Context, req any) (any, error) {
		return contentcount.Response{Counts: map[string]int{pid: 3}}, nil
	})
	e.b.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, func(_ context.Context, req any) (any, error) {
		return contentcount.Response{Counts: map[string]int{pid: 1}}, nil
	})
	if w := e.del(owner, "developer", pid, nameBody("Counted")); w.Code != 204 {
		t.Fatal(w.Code)
	}
	cm := e.auditList()[0].Args["counts"].(map[string]any)
	if cm["files"] != 3 || cm["specs"] != 1 || cm["keys"] != nil {
		t.Fatalf("%+v", cm)
	}
}

func TestDelete_CountsAllUnavailable(t *testing.T) {
	e := setup(t)
	owner := uuid.NewString()
	pid := e.project(t, "NoCounts", map[string]string{owner: "owner"})
	if w := e.del(owner, "developer", pid, nameBody("NoCounts")); w.Code != 204 {
		t.Fatal(w.Code)
	}
	cm := e.auditList()[0].Args["counts"].(map[string]any)
	if cm["files"] != nil || cm["specs"] != nil || cm["keys"] != nil {
		t.Fatalf("%+v", cm)
	}
}

func TestDelete_AuditFailOpen(t *testing.T) {
	d := migrated(t)
	for _, register := range []bool{true, false} {
		b := bus.NewRegistry(nil)
		if register {
			b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) { return nil, errors.New("audit down") })
		}
		api := chi.NewRouter()
		Init(app.Deps{Ctx: context.Background(), DB: d, Bus: b, API: api})
		var pid string
		_ = d.GetContext(context.Background(), &pid, `INSERT INTO projects (name) VALUES ('FailOpen') RETURNING id::text`)
		owner := uuid.NewString()
		_, _ = d.ExecContext(context.Background(), `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,'owner')`, pid, owner)
		req := httptest.NewRequest(http.MethodDelete, "/projects/"+pid, strings.NewReader(nameBody("FailOpen")))
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: owner, Role: "developer"}))
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		var n int
		_ = d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects WHERE id = $1`, pid)
		if w.Code != 204 || n != 0 {
			t.Fatalf("register=%v: %d n=%d", register, w.Code, n)
		}
	}
}

func TestDelete_ConcurrentDeletes(t *testing.T) {
	e := setup(t)
	owner := uuid.NewString()
	pid := e.project(t, "Racy", map[string]string{owner: "owner"})
	codes := make([]int, 5)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = e.del(owner, "developer", pid, nameBody("Racy")).Code
		}(i)
	}
	wg.Wait()
	ok, nf := 0, 0
	for _, c := range codes {
		switch c {
		case 204:
			ok++
		case 404:
			nf++
		}
	}
	if ok != 1 || nf != 4 || len(e.auditList()) != 1 {
		t.Fatalf("%v audit=%d", codes, len(e.auditList()))
	}
}

// holdLock menjalankan mutate di dalam tx uji yang memegang lock baris project, memicu request,
// menunggu sampai ada sesi yang menunggu lock (pg_stat_activity, maks 2 detik), lalu commit.
func holdLock(t *testing.T, e *env, pid string, mutate func(tx *sqlx.Tx) error, trigger func() <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	var ch <-chan *httptest.ResponseRecorder
	err := e.d.BeginTx(context.Background(), func(tx *sqlx.Tx) error {
		var x string
		if err := tx.GetContext(context.Background(), &x, `SELECT id::text FROM projects WHERE id = $1 FOR UPDATE`, pid); err != nil {
			return err
		}
		if err := mutate(tx); err != nil {
			return err
		}
		ch = trigger()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			var n int
			_ = e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock'`)
			if n > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return <-ch
}

func TestDelete_RenameRaceRejectsStaleName(t *testing.T) {
	e := setup(t)
	owner := uuid.NewString()
	pid := e.project(t, "Lama", map[string]string{owner: "owner"})
	w := holdLock(t, e, pid,
		func(tx *sqlx.Tx) error {
			_, err := tx.ExecContext(context.Background(), `UPDATE projects SET name = 'Baru' WHERE id = $1`, pid)
			return err
		},
		func() <-chan *httptest.ResponseRecorder {
			ch := make(chan *httptest.ResponseRecorder, 1)
			go func() { ch <- e.del(owner, "developer", pid, nameBody("Lama")) }()
			return ch
		})
	if b := parse(t, w); w.Code != 400 || b.Error.Details["reason"] != "CONFIRMATION_MISMATCH" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !e.exists(t, pid) || len(e.auditList()) != 0 {
		t.Fatal("project harus tetap ada")
	}
	if w := e.del(owner, "developer", pid, nameBody("Baru")); w.Code != 204 {
		t.Fatal(w.Code)
	}
}

func TestDelete_DemotedWhileWaiting(t *testing.T) {
	e := setup(t)
	owner, other := uuid.NewString(), uuid.NewString()
	pid := e.project(t, "Demote", map[string]string{owner: "owner", other: "owner"})
	w := holdLock(t, e, pid,
		func(tx *sqlx.Tx) error {
			_, err := tx.ExecContext(context.Background(), `UPDATE project_members SET role = 'member' WHERE project_id = $1 AND user_id = $2`, pid, owner)
			return err
		},
		func() <-chan *httptest.ResponseRecorder {
			ch := make(chan *httptest.ResponseRecorder, 1)
			go func() { ch <- e.del(owner, "developer", pid, nameBody("Demote")) }()
			return ch
		})
	if w.Code != 403 || !e.exists(t, pid) || len(e.auditList()) != 0 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestDelete_AuditArgsSnapshot(t *testing.T) {
	e := setup(t)
	owner := uuid.NewString()
	name := `Proyek "Ä" 日本`
	pid := e.project(t, name, map[string]string{owner: "owner"})
	if w := e.del(owner, "developer", pid, nameBody(name)); w.Code != 204 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	a := e.auditList()[0]
	if a.ProjectLabel != name || a.Args["name"] != name {
		t.Fatalf("%+v", a)
	}
}

func TestInit_RegistersRoute(t *testing.T) {
	e := setup(t)
	n := 0
	_ = chi.Walk(e.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodDelete && route == "/api/projects/{projectID}" {
			n++
		}
		return nil
	})
	if n != 1 {
		t.Fatalf("route terdaftar %d kali", n)
	}
}
