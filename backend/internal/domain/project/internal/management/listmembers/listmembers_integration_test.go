//go:build integration

package listmembers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/getuser"
)

type env struct {
	d     *db.DB
	h     http.Handler
	calls int
}

type body struct {
	Error *struct{ Code, Message string } `json:"error"`
	Items []struct {
		UserID   string  `json:"user_id"`
		Name     *string `json:"name"`
		IsActive bool    `json:"is_active"`
		Found    bool    `json:"found"`
		Role     string  `json:"role"`
	} `json:"items"`
}

func setup(t *testing.T, people map[string]getuser.Response, failFor string, registered bool) *env {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatal(err)
	}
	e := &env{d: d}
	b := bus.NewRegistry(nil)
	if registered {
		b.Register(constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
			e.calls++
			id := req.(getuser.Request).UserID
			if id == failFor {
				return nil, errors.New("boom")
			}
			return people[id], nil
		})
	}
	s := store.New(d)
	r := chi.NewRouter()
	r.Get("/projects/{projectID}/members", newHandler(NewUsecase(s, s, users.New(b))).ServeHTTP)
	e.h = r
	return e
}

func (e *env) get(uid, role, pid string) (int, body) {
	req := httptest.NewRequest(http.MethodGet, "/projects/"+pid+"/members", nil)
	req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: uid, Role: role}))
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	var b body
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return w.Code, b
}

func (e *env) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.d.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	pid               string
	a, b, dd, x, c, s string
	people            map[string]getuser.Response
}

func newFixture() fixture {
	f := fixture{a: uuid.NewString(), b: uuid.NewString(), dd: uuid.NewString(), x: uuid.NewString(), c: uuid.NewString(), s: uuid.NewString()}
	mk := func(id, n string, act bool) getuser.Response {
		return getuser.Response{Found: true, ID: id, Name: n, Email: n + "@x.id", Role: "developer", IsActive: act}
	}
	f.people = map[string]getuser.Response{f.a: mk(f.a, "Andi", true), f.b: mk(f.b, "Budi", true), f.dd: mk(f.dd, "Dedi", false)}
	return f
}

func (f *fixture) seed(t *testing.T, e *env) {
	t.Helper()
	if err := e.d.GetContext(context.Background(), &f.pid, `INSERT INTO projects (name) VALUES ('Project') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	for _, m := range [][2]string{{f.a, "owner"}, {f.b, "member"}, {f.dd, "member"}, {f.x, "member"}} {
		e.exec(t, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, f.pid, m[0], m[1])
	}
}

func TestT03_AC2_MemberCanSeeMembers(t *testing.T) {
	f := newFixture()
	e := setup(t, f.people, "", true)
	f.seed(t, e)
	code, b := e.get(f.b, "developer", f.pid)
	if code != 200 || len(b.Items) != 4 || b.Items[0].UserID != f.a || b.Items[0].Role != "owner" {
		t.Fatalf("%d %+v", code, b)
	}
	for _, it := range b.Items {
		switch it.UserID {
		case f.dd:
			if it.IsActive || !it.Found {
				t.Fatalf("dd %+v", it)
			}
		case f.x:
			if it.Found || it.Name != nil || it.IsActive {
				t.Fatalf("x %+v", it)
			}
		}
	}
	if b.Items[3].UserID != f.x {
		t.Fatalf("unknown member should sort last: %+v", b.Items)
	}
}

func TestListMembers_SuperAdminSeesAll(t *testing.T) {
	f := newFixture()
	e := setup(t, f.people, "", true)
	f.seed(t, e)
	code, b := e.get(f.s, "super_admin", f.pid)
	if code != 200 || len(b.Items) != 4 || e.calls != 4 {
		t.Fatalf("%d %d %d", code, len(b.Items), e.calls)
	}
}

func TestT03_Security_ProjectIsolation_ListMembers(t *testing.T) {
	f := newFixture()
	e := setup(t, f.people, "", true)
	f.seed(t, e)
	var msgs []string
	for _, pid := range []string{f.pid, uuid.NewString(), "bukan-uuid"} {
		code, b := e.get(f.c, "developer", pid)
		if code != 404 || b.Error == nil || b.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s: %d %+v", pid, code, b)
		}
		msgs = append(msgs, b.Error.Message)
	}
	if msgs[0] != msgs[1] || msgs[1] != msgs[2] || e.calls != 0 {
		t.Fatalf("%v %d", msgs, e.calls)
	}
}

func TestListMembers_GetUserFailures(t *testing.T) {
	f := newFixture()
	e := setup(t, f.people, "", false)
	f.seed(t, e)
	if code, _ := e.get(f.s, "super_admin", f.pid); code != 500 {
		t.Fatalf("unregistered: %d", code)
	}
	e2 := setup(t, f.people, f.b, true)
	f.seed(t, e2)
	if code, b := e2.get(f.a, "developer", f.pid); code != 500 || len(b.Items) != 0 {
		t.Fatalf("one fails: %d %+v", code, b)
	}
}

func TestListMembers_ProjectWithoutMembers(t *testing.T) {
	f := newFixture()
	e := setup(t, f.people, "", true)
	if err := e.d.GetContext(context.Background(), &f.pid, `INSERT INTO projects (name) VALUES ('Empty') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	code, b := e.get(f.s, "super_admin", f.pid)
	if code != 200 || b.Items == nil || len(b.Items) != 0 {
		t.Fatalf("%d %+v", code, b)
	}
}

func TestListMembers_AfterMemberRemoved(t *testing.T) {
	f := newFixture()
	e := setup(t, f.people, "", true)
	f.seed(t, e)
	e.exec(t, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, f.pid, f.b)
	if code, _ := e.get(f.b, "developer", f.pid); code != 404 {
		t.Fatalf("%d", code)
	}
	code, b := e.get(f.a, "developer", f.pid)
	if code != 200 || len(b.Items) != 3 {
		t.Fatalf("%d %+v", code, b)
	}
	for _, it := range b.Items {
		if it.UserID == f.b {
			t.Fatal("removed member still listed")
		}
	}
}
