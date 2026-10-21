//go:build integration

package getproject

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

func migrated(t *testing.T) *db.DB {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func insertProject(t *testing.T, d *db.DB, name string) string {
	t.Helper()
	var id string
	if err := d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func addMember(t *testing.T, d *db.DB, pid, uid, role string) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, pid, uid, role); err != nil {
		t.Fatal(err)
	}
}

func TestRepo_MemberStats(t *testing.T) {
	d := migrated(t)
	r := newRepo(d, store.New(d))
	ctx := context.Background()
	p0, p1, p2, other := insertProject(t, d, "p0"), insertProject(t, d, "p1"), insertProject(t, d, "p2"), insertProject(t, d, "other")
	addMember(t, d, p1, uuid.NewString(), "owner")
	addMember(t, d, p1, uuid.NewString(), "member")
	addMember(t, d, p1, uuid.NewString(), "member")
	addMember(t, d, p2, uuid.NewString(), "owner")
	addMember(t, d, p2, uuid.NewString(), "owner")
	addMember(t, d, other, uuid.NewString(), "owner")
	for _, c := range []struct {
		id   string
		m, o int
	}{{p0, 0, 0}, {p1, 3, 1}, {p2, 2, 2}} {
		m, o, err := r.MemberStats(ctx, c.id)
		if err != nil || m != c.m || o != c.o {
			t.Fatalf("%s: %d %d %v", c.id, m, o, err)
		}
	}
}

func TestRepo_FindAccess_Delegates(t *testing.T) {
	d := migrated(t)
	r := newRepo(d, store.New(d))
	pid, u := insertProject(t, d, "Project"), uuid.NewString()
	addMember(t, d, pid, u, "member")
	_, role, found, err := r.FindAccess(context.Background(), pid, u)
	if err != nil || !found || role != "member" {
		t.Fatalf("%q %v %v", role, found, err)
	}
	_, role, found, err = r.FindAccess(context.Background(), pid, uuid.NewString())
	if err != nil || !found || role != "" {
		t.Fatalf("%q %v %v", role, found, err)
	}
}

type env struct {
	d *db.DB
	b *bus.Registry
	h http.Handler
}

func setup(t *testing.T) *env {
	t.Helper()
	d := migrated(t)
	b := bus.NewRegistry(nil)
	uc := NewUsecase(newRepo(d, store.New(d)), counts.New(b, nil))
	r := chi.NewRouter()
	r.Get("/projects/{projectID}", newHandler(uc).ServeHTTP)
	return &env{d: d, b: b, h: r}
}

func (e *env) get(uid, role, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/projects/"+id, nil)
	req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: uid, Role: role}))
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

type body struct {
	Error *struct {
		Code, Message string
		Details       any
	} `json:"error"`
	MyRole      *string        `json:"my_role"`
	AccessLevel string         `json:"access_level"`
	IsMember    bool           `json:"is_member"`
	MemberCount int            `json:"member_count"`
	OwnerCount  int            `json:"owner_count"`
	Counts      map[string]any `json:"counts"`
}

func decode(t *testing.T, w *httptest.ResponseRecorder) body {
	t.Helper()
	var b body
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestT03_AC1_GetProject_NonMemberGets404(t *testing.T) {
	e := setup(t)
	pid, a, b := insertProject(t, e.d, "Project"), uuid.NewString(), uuid.NewString()
	addMember(t, e.d, pid, a, "owner")
	var sigs []body
	for _, id := range []string{pid, uuid.NewString(), "bukan-uuid"} {
		w := e.get(b, "developer", id)
		if w.Code != 404 {
			t.Fatalf("%s: %d", id, w.Code)
		}
		sigs = append(sigs, decode(t, w))
	}
	for _, s := range sigs[1:] {
		if *s.Error != *sigs[0].Error {
			t.Fatalf("beda: %+v vs %+v", s.Error, sigs[0].Error)
		}
	}
}

func TestT03_AC2_GetProject_MemberSees(t *testing.T) {
	e := setup(t)
	pid, a, b := insertProject(t, e.d, "Project"), uuid.NewString(), uuid.NewString()
	addMember(t, e.d, pid, a, "owner")
	addMember(t, e.d, pid, b, "member")
	w := e.get(b, "developer", pid)
	r := decode(t, w)
	if w.Code != 200 || r.MyRole == nil || *r.MyRole != "member" || r.AccessLevel != "member" || r.MemberCount != 2 || r.OwnerCount != 1 {
		t.Fatalf("%d %+v", w.Code, r)
	}
}

func TestT03_AC7_GetProject_SuperAdminSeesAny(t *testing.T) {
	e := setup(t)
	pid := insertProject(t, e.d, "Project")
	w := e.get(uuid.NewString(), "super_admin", pid)
	r := decode(t, w)
	if w.Code != 200 || r.AccessLevel != "admin" || r.MyRole != nil || r.IsMember || r.MemberCount != 0 || r.OwnerCount != 0 {
		t.Fatalf("%d %+v", w.Code, r)
	}
}

func TestT03_GetProject_CountsFailSoft(t *testing.T) {
	e := setup(t)
	pid := insertProject(t, e.d, "Project")
	s := uuid.NewString()
	r := decode(t, e.get(s, "super_admin", pid))
	for _, k := range []string{"files", "specs", "keys"} {
		if v, ok := r.Counts[k]; !ok || v != nil {
			t.Fatalf("%s: %v", k, r.Counts)
		}
	}
	e.b.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, func(_ context.Context, req any) (any, error) {
		return contentcount.Response{Counts: map[string]int{pid: 3}}, nil
	})
	e.b.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, func(context.Context, any) (any, error) {
		return nil, context.DeadlineExceeded
	})
	w := e.get(s, "super_admin", pid)
	r = decode(t, w)
	if w.Code != 200 || r.Counts["files"] != float64(3) || r.Counts["specs"] != nil || r.Counts["keys"] != nil {
		t.Fatalf("%d %v", w.Code, r.Counts)
	}
}

func TestT03_Security_GetProject_Isolation(t *testing.T) {
	e := setup(t)
	pa, pc := insertProject(t, e.d, "Alpha"), insertProject(t, e.d, "Charlie")
	c := uuid.NewString()
	addMember(t, e.d, pc, c, "owner")
	w := e.get(c, "developer", pa)
	if w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
	if r := decode(t, w); r.Error == nil || r.Error.Code != "NOT_FOUND" {
		t.Fatalf("%s", w.Body.String())
	}
}
