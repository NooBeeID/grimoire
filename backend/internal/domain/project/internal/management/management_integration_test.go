//go:build integration

package management

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/internal/infra/middleware"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/project/management/checkaccess"
	"grimoire/lib/contracts/project/management/contentcount"
	"grimoire/lib/contracts/project/management/getproject"
)

const (
	roleDev   = "developer"
	roleAdmin = "super_admin"
)

type fakeUser struct {
	getuser.Response
}

type env struct {
	t   *testing.T
	d   *db.DB
	b   *bus.Registry
	h   http.Handler
	api chi.Router
	top chi.Router

	mu         sync.Mutex
	users      map[string]*fakeUser
	audits     []record.Request
	getUserHit map[string]int
	auditFail  atomic.Bool
	finderFail atomic.Bool
	finderReqs []finddevelopers.Request

	A, B, C, D, S string
}

// registerIfAbsent hanya mendaftar di registry test ini (bukan registry produksi).
func registerIfAbsent(b *bus.Registry, endpoint string, h bus.Handler) {
	if _, err := b.Call(context.Background(), endpoint, nil); errors.Is(err, bus.ErrNotRegistered) {
		b.Register(endpoint, h)
	}
}

type setupOpts struct {
	noFinder, noGetUser, noAudit bool
}

func setup(t *testing.T) *env { return setupWith(t, setupOpts{}) }

func setupWith(t *testing.T, o setupOpts) *env {
	t.Helper()
	d := db.TestDB(t)
	ctx := context.Background()
	if _, err := db.Up(ctx, d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := d.ExecContext(ctx, `CREATE TABLE project_child (id uuid primary key default gen_random_uuid(), project_id uuid not null references projects(id) on delete cascade)`); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, d: d, b: bus.NewRegistry(nil), users: map[string]*fakeUser{}, getUserHit: map[string]int{}}
	e.A, e.B, e.C, e.D, e.S = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.addUser(e.A, "Alice", roleDev, true)
	e.addUser(e.B, "Bob", roleDev, true)
	e.addUser(e.C, "Carol", roleDev, true)
	e.addUser(e.D, "Dave", roleDev, false)
	e.addUser(e.S, "Root", roleAdmin, true)

	if !o.noGetUser {
		registerIfAbsent(e.b, constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
			r := req.(getuser.Request)
			e.mu.Lock()
			defer e.mu.Unlock()
			e.getUserHit[r.UserID]++
			u, ok := e.users[r.UserID]
			if !ok {
				return getuser.Response{}, nil
			}
			return u.Response, nil
		})
	}
	if !o.noFinder {
		registerIfAbsent(e.b, constants.BUS_CORE_AUTH_FIND_DEVELOPERS, func(_ context.Context, req any) (any, error) {
			r := req.(finddevelopers.Request)
			e.mu.Lock()
			defer e.mu.Unlock()
			e.finderReqs = append(e.finderReqs, r)
			if e.finderFail.Load() {
				return nil, errors.New("finder down")
			}
			var items []finddevelopers.Developer
			for _, u := range e.users {
				if u.Role != roleDev || !u.IsActive {
					continue
				}
				q := strings.ToLower(r.Query)
				if q == "" || strings.Contains(strings.ToLower(u.Name), q) || strings.Contains(strings.ToLower(u.Email), q) {
					items = append(items, finddevelopers.Developer{ID: u.ID, Name: u.Name, Email: u.Email})
				}
			}
			sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
			return finddevelopers.Response{Items: items}, nil
		})
	}
	if !o.noAudit {
		registerIfAbsent(e.b, constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
			if e.auditFail.Load() {
				return nil, errors.New("audit down")
			}
			e.mu.Lock()
			defer e.mu.Unlock()
			e.audits = append(e.audits, req.(record.Request))
			return record.Response{Recorded: true}, nil
		})
	}

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
	Register(app.Deps{Ctx: ctx, DB: d, Bus: e.b, API: api})
	r.Mount("/api", api)
	e.api, e.top, e.h = api, r, r
	return e
}

func (e *env) addUser(id, name, role string, active bool) {
	e.users[id] = &fakeUser{getuser.Response{Found: true, ID: id, Name: name, Email: strings.ToLower(name) + "@x.id", Role: role, IsActive: active}}
}

func (e *env) setActive(id string, active bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.users[id].IsActive = active
}

func (e *env) roleOf(uid string) string {
	if uid == e.S {
		return roleAdmin
	}
	return roleDev
}

func (e *env) auditList() []record.Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]record.Request(nil), e.audits...)
}

func (e *env) auditActions(action string) []record.Request {
	var out []record.Request
	for _, a := range e.auditList() {
		if a.Action == action {
			out = append(out, a)
		}
	}
	return out
}

func (e *env) do(uid, method, path string, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	switch v := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(v))
	default:
		b, _ := json.Marshal(v)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, "/api"+path, rd)
	req.Header.Set("X-Requested-With", "grimoire")
	req.Header.Set("Content-Type", "application/json")
	if uid != "" {
		req.Header.Set("X-Test-Uid", uid)
		req.Header.Set("X-Test-Role", e.roleOf(uid))
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

type errBody struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func parseErr(w *httptest.ResponseRecorder) errBody {
	var b errBody
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b
}

func mustCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
	}
}

func mustReason(t *testing.T, w *httptest.ResponseRecorder, status int, reason string) {
	t.Helper()
	mustCode(t, w, status)
	if got := parseErr(w).Error.Details["reason"]; got != reason {
		t.Fatalf("reason %v, want %s: %s", got, reason, w.Body.String())
	}
}

func (e *env) create(uid, name string) base.ProjectDetail {
	e.t.Helper()
	w := e.do(uid, http.MethodPost, "/projects", map[string]string{"name": name, "description": "d"})
	mustCode(e.t, w, 201)
	var p base.ProjectDetail
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) addMember(actor, pid, user, role string) *httptest.ResponseRecorder {
	return e.do(actor, http.MethodPost, "/projects/"+pid+"/members", map[string]string{"user_id": user, "role": role})
}

func (e *env) list(uid string) []base.ProjectSummary {
	e.t.Helper()
	w := e.do(uid, http.MethodGet, "/projects", nil)
	mustCode(e.t, w, 200)
	var out struct {
		Items []base.ProjectSummary `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	return out.Items
}

func (e *env) detail(uid, pid string) base.ProjectDetail {
	e.t.Helper()
	w := e.do(uid, http.MethodGet, "/projects/"+pid, nil)
	mustCode(e.t, w, 200)
	var p base.ProjectDetail
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) count(table, pid string) int {
	e.t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM `+table+` WHERE project_id = $1`, pid); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) ownerCount(pid string) int {
	e.t.Helper()
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`, pid); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func hasProject(items []base.ProjectSummary, id string) bool {
	for _, p := range items {
		if p.ID == id {
			return true
		}
	}
	return false
}

func TestT03_AC1_CreatorIsOwnerAndInvisibleToOthers(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	if p.MyRole == nil || *p.MyRole != "owner" || p.MemberCount != 1 {
		t.Fatalf("%+v", p)
	}
	if hasProject(e.list(e.B), p.ID) {
		t.Fatal("B tidak boleh melihat project A")
	}
	w := e.do(e.B, http.MethodGet, "/projects/"+p.ID, nil)
	if b := parseErr(w); w.Code != 404 || b.Error.Message != "project tidak ditemukan" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestT03_AC2_MemberCanEditButNotManage(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	if !hasProject(e.list(e.B), p.ID) {
		t.Fatal("B harus melihat project")
	}
	mustCode(t, e.do(e.B, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Alpha 2"}), 200)
	mustCode(t, e.addMember(e.B, p.ID, e.C, "member"), 403)
	mustCode(t, e.do(e.B, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.A, nil), 403)
	mustCode(t, e.do(e.B, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.A, map[string]string{"role": "member"}), 403)
	mustCode(t, e.do(e.B, http.MethodDelete, "/projects/"+p.ID, map[string]string{"confirm_name": "Alpha 2"}), 403)
	mustCode(t, e.do(e.B, http.MethodGet, "/projects/"+p.ID+"/member-candidates?q=c", nil), 403)
	if e.count("project_members", p.ID) != 2 {
		t.Fatal("anggota berubah")
	}
}

func TestT03_AC3_LastOwnerCannotLeaveOrDemoteSelf(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustReason(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.A, nil), 409, "LAST_OWNER")
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.A, map[string]string{"role": "member"}), 409)
	mustCode(t, e.do(e.S, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.A, map[string]string{"role": "member"}), 200)
	if d := e.detail(e.S, p.ID); d.OwnerCount != 0 {
		t.Fatalf("owner_count=%d", d.OwnerCount)
	}
	mustCode(t, e.do(e.S, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.A, nil), 204)
}

func TestT03_AC3_ConcurrentMutualDemotion(t *testing.T) {
	e := setup(t)
	for i := 0; i < 20; i++ {
		p := e.create(e.A, "Race")
		mustCode(t, e.addMember(e.A, p.ID, e.C, "owner"), 201)
		codes := make([]int, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			codes[0] = e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.C, map[string]string{"role": "member"}).Code
		}()
		go func() {
			defer wg.Done()
			codes[1] = e.do(e.C, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.A, map[string]string{"role": "member"}).Code
		}()
		wg.Wait()
		ok, forbidden := 0, 0
		for _, c := range codes {
			switch c {
			case 200:
				ok++
			case 403:
				forbidden++
			}
		}
		if ok != 1 || forbidden != 1 || e.ownerCount(p.ID) != 1 {
			t.Fatalf("putaran %d: codes=%v owners=%d", i, codes, e.ownerCount(p.ID))
		}
	}
}

func TestT03_AC4_RejectIneligibleCandidates(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustReason(t, e.addMember(e.A, p.ID, e.D, "member"), 400, "USER_INACTIVE")
	mustReason(t, e.addMember(e.A, p.ID, e.S, "member"), 400, "USER_NOT_DEVELOPER")
	mustReason(t, e.addMember(e.A, p.ID, uuid.NewString(), "member"), 400, "USER_NOT_FOUND")
	mustReason(t, e.addMember(e.A, p.ID, "bukan-uuid", "member"), 400, "USER_NOT_FOUND")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	mustReason(t, e.addMember(e.A, p.ID, e.B, "member"), 409, "ALREADY_MEMBER")
	if e.count("project_members", p.ID) != 2 {
		t.Fatal("jumlah anggota salah")
	}
}

func TestT03_AC5_DeactivatedMemberHasNoAccess(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Alpha")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	e.setActive(e.B, false)

	call := func(uid, role string) checkaccess.Response {
		t.Helper()
		got, err := bus.CallTyped[checkaccess.Response](context.Background(), e.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
			checkaccess.Request{ProjectID: p.ID, UserID: uid, UserRole: role})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := call(e.B, roleDev); !got.ProjectExists || got.Level != checkaccess.LevelNone {
		t.Fatalf("%+v", got)
	}
	if got := call(e.A, roleDev); got.Level != checkaccess.LevelOwner {
		t.Fatalf("%+v", got)
	}
	e.mu.Lock()
	before := e.getUserHit[e.S]
	e.mu.Unlock()
	if got := call(e.S, roleAdmin); got.Level != checkaccess.LevelAdmin {
		t.Fatalf("%+v", got)
	}
	e.mu.Lock()
	after := e.getUserHit[e.S]
	e.mu.Unlock()
	if before != after {
		t.Fatal("super_admin tidak boleh memicu get-user")
	}
}

func TestT03_AC6_DeleteRequiresConfirmationAndCascades(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Payments v2")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	if _, err := e.d.ExecContext(context.Background(), `INSERT INTO project_child (project_id) VALUES ($1)`, p.ID); err != nil {
		t.Fatal(err)
	}
	mustReason(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID, map[string]string{"confirm_name": "salah"}), 400, "CONFIRMATION_MISMATCH")
	mustCode(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID, nil), 400)
	var n int
	_ = e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects WHERE id = $1`, p.ID)
	if n != 1 || e.count("project_members", p.ID) != 2 || e.count("project_child", p.ID) != 1 {
		t.Fatal("data berubah setelah penolakan")
	}
	mustCode(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID, map[string]string{"confirm_name": "Payments v2"}), 204)
	_ = e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects WHERE id = $1`, p.ID)
	if n != 0 || e.count("project_members", p.ID) != 0 || e.count("project_child", p.ID) != 0 {
		t.Fatal("cascade gagal")
	}
	as := e.auditActions(record.ActionProjectDeleted)
	if len(as) != 1 {
		t.Fatalf("audit: %d", len(as))
	}
	a := as[0]
	if a.Args["name"] != "Payments v2" || a.Args["member_count"] != 2 {
		t.Fatalf("%+v", a.Args)
	}
	if _, ok := a.Args["counts"]; !ok {
		t.Fatalf("args.counts hilang: %+v", a.Args)
	}
}

func TestT03_AC6_AllProjectFKsCascade(t *testing.T) {
	e := setup(t)
	var rows []struct {
		Rel  string `db:"rel"`
		Type string `db:"typ"`
	}
	if err := e.d.SelectContext(context.Background(), &rows, `SELECT conrelid::regclass::text AS rel, confdeltype::text AS typ FROM pg_constraint WHERE contype = 'f' AND confrelid = 'projects'::regclass`); err != nil {
		t.Fatal(err)
	}
	if len(rows) < 1 {
		t.Fatal("minimal satu FK (project_members) diharapkan")
	}
	for _, r := range rows {
		if r.Type != "c" {
			t.Errorf("FK %s ke projects(id) tanpa ON DELETE CASCADE", r.Rel)
		}
	}
}

func TestT03_AC7_SuperAdminSeesAndManagesAll(t *testing.T) {
	e := setup(t)
	pa := e.create(e.A, "Proj A")
	pc := e.create(e.C, "Proj C")
	items := e.list(e.S)
	for _, id := range []string{pa.ID, pc.ID} {
		found := false
		for _, it := range items {
			if it.ID == id {
				found = true
				if it.AccessLevel != "admin" || it.IsMember || it.MyRole != nil {
					t.Fatalf("%+v", it)
				}
			}
		}
		if !found {
			t.Fatalf("S tidak melihat %s", id)
		}
	}
	w := e.do(e.S, http.MethodPost, "/projects", map[string]string{"name": "Admin Proj"})
	mustCode(t, w, 201)
	var sp base.ProjectDetail
	_ = json.Unmarshal(w.Body.Bytes(), &sp)
	if sp.MemberCount != 0 {
		t.Fatalf("member_count=%d", sp.MemberCount)
	}
	mustCode(t, e.addMember(e.S, sp.ID, e.A, "owner"), 201)
	mustCode(t, e.addMember(e.A, pc.ID, e.B, "member"), 404)
}

func TestT03_AC7_SuperAdminManagesOtherProjectMembers(t *testing.T) {
	e := setup(t)
	pc := e.create(e.C, "Proj C")
	mustCode(t, e.addMember(e.C, pc.ID, e.B, "member"), 201)
	mustCode(t, e.do(e.S, http.MethodPatch, "/projects/"+pc.ID+"/members/"+e.B, map[string]string{"role": "owner"}), 200)
	mustCode(t, e.do(e.S, http.MethodDelete, "/projects/"+pc.ID+"/members/"+e.B, nil), 204)
}

func TestT03_AC8_CheckAccessUnknownProject(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for _, id := range []string{uuid.NewString(), "bukan-uuid"} {
		got, err := bus.CallTyped[checkaccess.Response](ctx, e.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
			checkaccess.Request{ProjectID: id, UserID: e.A, UserRole: roleDev})
		if err != nil || got.ProjectExists || got.Level != checkaccess.LevelNone {
			t.Fatalf("%s: %+v %v", id, got, err)
		}
	}
	if _, err := e.b.Call(ctx, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, "salah"); !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
	_, err := e.b.Call(ctx, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, checkaccess.Request{ProjectID: uuid.NewString(), UserID: e.A, UserRole: "agent"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestT03_Security_ProjectIsolation(t *testing.T) {
	e := setup(t)
	pa := e.create(e.A, "Secret PA")
	mustCode(t, e.addMember(e.A, pa.ID, e.C, "member"), 201)

	type route struct {
		method, path string
		body         any
	}
	build := func(id string) []route {
		return []route{
			{http.MethodPost, "/projects/" + id + "/members", map[string]string{"user_id": e.C, "role": "member"}},
			{http.MethodGet, "/projects/" + id, nil},
			{http.MethodPatch, "/projects/" + id, map[string]string{"name": "Hacked"}},
			{http.MethodDelete, "/projects/" + id, map[string]string{"confirm_name": "Secret PA"}},
			{http.MethodGet, "/projects/" + id + "/members", nil},
			{http.MethodGet, "/projects/" + id + "/member-candidates?q=a", nil},
			{http.MethodDelete, "/projects/" + id + "/members/" + e.C, nil},
			{http.MethodPatch, "/projects/" + id + "/members/" + e.C, map[string]string{"role": "owner"}},
		}
	}
	var first string
	for _, id := range []string{pa.ID, uuid.NewString(), "bukan-uuid"} {
		for _, r := range build(id) {
			w := e.do(e.B, r.method, r.path, r.body)
			b := parseErr(w)
			if w.Code != 404 || b.Error.Code != "NOT_FOUND" || b.Error.Message != "project tidak ditemukan" || b.Error.Details != nil {
				t.Fatalf("%s %s: %d %s", r.method, r.path, w.Code, w.Body.String())
			}
			var m map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &m)
			delete(m, "trace_id")
			out, _ := json.Marshal(m)
			if first == "" {
				first = string(out)
			} else if string(out) != first {
				t.Fatalf("body tidak seragam: %s vs %s", out, first)
			}
		}
	}
	// route ke-10: POST /projects tidak terikat project; GET /projects tidak memuat PA.
	if hasProject(e.list(e.B), pa.ID) {
		t.Fatal("list bocor")
	}
	d := e.detail(e.A, pa.ID)
	if d.Name != "Secret PA" || d.MemberCount != 2 {
		t.Fatalf("data PA berubah: %+v", d)
	}
	got, err := bus.CallTyped[checkaccess.Response](context.Background(), e.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
		checkaccess.Request{ProjectID: pa.ID, UserID: e.B, UserRole: roleDev})
	if err != nil || got.Level != checkaccess.LevelNone {
		t.Fatalf("%+v %v", got, err)
	}
	gp, err := bus.CallTyped[getproject.Response](context.Background(), e.b, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: pa.ID})
	if err != nil || !gp.Found || gp.ID != pa.ID || gp.Name != "Secret PA" {
		t.Fatalf("%+v %v", gp, err)
	}
}

func TestT03_Audit_OneEntryPerAction(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Audited")
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Audited 2"}), 200)
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.B, map[string]string{"role": "owner"}), 200)
	mustCode(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID+"/members/"+e.B, nil), 204)
	// no-op: tidak mencatat
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID, map[string]string{"name": "Audited 2"}), 200)
	mustCode(t, e.addMember(e.A, p.ID, e.C, "member"), 201)
	mustCode(t, e.do(e.A, http.MethodPatch, "/projects/"+p.ID+"/members/"+e.C, map[string]string{"role": "member"}), 200)
	mustCode(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID, map[string]string{"confirm_name": "Audited 2"}), 204)

	want := map[string]int{
		record.ActionProjectCreated:           1,
		record.ActionProjectUpdated:           1,
		record.ActionProjectMemberAdded:       2,
		record.ActionProjectMemberRoleChanged: 1,
		record.ActionProjectMemberRemoved:     1,
		record.ActionProjectDeleted:           1,
	}
	for action, n := range want {
		got := e.auditActions(action)
		if len(got) != n {
			t.Fatalf("%s: %d entri, want %d", action, len(got), n)
		}
		for _, a := range got {
			if a.ProjectID != p.ID || a.ActorID != e.A || a.ActorType != record.ActorUser || a.Status != record.StatusOK {
				t.Fatalf("%s: %+v", action, a)
			}
		}
	}
	if got := e.auditActions(record.ActionProjectUpdated)[0].ProjectLabel; got != "Audited 2" {
		t.Fatalf("label update: %q", got)
	}
	if got := e.auditActions(record.ActionProjectDeleted)[0].ProjectLabel; got != "Audited 2" {
		t.Fatalf("label delete: %q", got)
	}
	if len(e.auditList()) != 7 {
		t.Fatalf("total audit %d, want 7", len(e.auditList()))
	}
}

func TestT03_Audit_FailOpen(t *testing.T) {
	e := setup(t)
	e.auditFail.Store(true)
	p := e.create(e.A, "FailOpen")
	mustCode(t, e.addMember(e.A, p.ID, e.B, "member"), 201)
	mustCode(t, e.do(e.A, http.MethodDelete, "/projects/"+p.ID, map[string]string{"confirm_name": "FailOpen"}), 204)
	if len(e.auditList()) != 0 {
		t.Fatal("audit seharusnya kosong")
	}
}

func TestT03_Counts_FailSoft(t *testing.T) {
	e := setup(t)
	p := e.create(e.A, "Counted")
	d := e.detail(e.A, p.ID)
	if d.Counts.Files != nil || d.Counts.Specs != nil || d.Counts.Keys != nil {
		t.Fatalf("%+v", d.Counts)
	}
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, func(context.Context, any) (any, error) {
		return contentcount.Response{Counts: map[string]int{p.ID: 3}}, nil
	})
	d = e.detail(e.A, p.ID)
	if d.Counts.Files == nil || *d.Counts.Files != 3 || d.Counts.Specs != nil || d.Counts.Keys != nil {
		t.Fatalf("%+v", d.Counts)
	}
}

func TestRoutesRegistered(t *testing.T) {
	e := setup(t)
	want := []string{
		"POST /api/projects",
		"GET /api/projects",
		"GET /api/projects/{projectID}",
		"PATCH /api/projects/{projectID}",
		"DELETE /api/projects/{projectID}",
		"GET /api/projects/{projectID}/members",
		"GET /api/projects/{projectID}/member-candidates",
		"POST /api/projects/{projectID}/members",
		"DELETE /api/projects/{projectID}/members/{userID}",
		"PATCH /api/projects/{projectID}/members/{userID}",
	}
	var got []string
	err := chi.Walk(e.top, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+strings.TrimSuffix(route, "/"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %d route %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route tidak sama:\n got %v\nwant %v", got, want)
		}
	}

	ctx := context.Background()
	if _, err := e.b.Call(ctx, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, checkaccess.Request{ProjectID: uuid.NewString(), UserID: e.A, UserRole: roleDev}); errors.Is(err, bus.ErrNotRegistered) || err != nil {
		t.Fatalf("check-access: %v", err)
	}
	if _, err := e.b.Call(ctx, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: uuid.NewString()}); errors.Is(err, bus.ErrNotRegistered) || err != nil {
		t.Fatalf("get-project: %v", err)
	}
}
