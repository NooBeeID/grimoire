//go:build integration

package removemember

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/getuser"
)

type itAudit struct {
	mu sync.Mutex
	ev []audit.Event
}

func (a *itAudit) Record(_ context.Context, ev audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ev = append(a.ev, ev)
}

type itEnv struct {
	d *db.DB
	h http.Handler
	a *itAudit
}

func itSetup(t *testing.T, registered bool) *itEnv {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatal(err)
	}
	b := bus.NewRegistry(nil)
	if registered {
		b.Register(constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
			id := req.(getuser.Request).UserID
			return getuser.Response{Found: true, ID: id, Email: id[:4] + "@tim.id", Role: "developer", IsActive: true}, nil
		})
	}
	e := &itEnv{d: d, a: &itAudit{}}
	uc := NewUsecase(newRepo(d, store.New(d)), users.New(b), e.a, slog.Default())
	r := chi.NewRouter()
	r.Delete("/projects/{projectID}/members/{userID}", handler{uc: uc}.ServeHTTP)
	e.h = r
	return e
}

func (e *itEnv) del(uid, role, pid, target string) int {
	req := httptest.NewRequest(http.MethodDelete, "/projects/"+pid+"/members/"+target, nil)
	req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: uid, Role: role}))
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w.Code
}

func (e *itEnv) project(t *testing.T, members map[string]string) string {
	t.Helper()
	var pid string
	if err := e.d.GetContext(context.Background(), &pid, `INSERT INTO projects (name) VALUES ('Payments') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	for id, role := range members {
		if _, err := e.d.ExecContext(context.Background(), `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, pid, id, role); err != nil {
			t.Fatal(err)
		}
	}
	return pid
}

func (e *itEnv) count(t *testing.T, pid string) (members, owners int) {
	t.Helper()
	if err := e.d.GetContext(context.Background(), &members, `SELECT count(*) FROM project_members WHERE project_id=$1`, pid); err != nil {
		t.Fatal(err)
	}
	if err := e.d.GetContext(context.Background(), &owners, `SELECT count(*) FROM project_members WHERE project_id=$1 AND role='owner'`, pid); err != nil {
		t.Fatal(err)
	}
	return
}

func TestT03_AC2_MemberCannotRemoveOthers_HTTP(t *testing.T) {
	e := itSetup(t, true)
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner", b: "member", c: "member"})
	if code := e.del(b, "developer", pid, c); code != 403 {
		t.Fatalf("code %d", code)
	}
	if m, _ := e.count(t, pid); m != 3 || len(e.a.ev) != 0 {
		t.Fatalf("members %d audit %d", m, len(e.a.ev))
	}
}

func TestT03_AC2_MemberCanLeave(t *testing.T) {
	e := itSetup(t, true)
	a, b := uuid.NewString(), uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	if code := e.del(b, "developer", pid, b); code != 204 {
		t.Fatalf("code %d", code)
	}
	if m, _ := e.count(t, pid); m != 1 {
		t.Fatalf("members %d", m)
	}
	if len(e.a.ev) != 1 || e.a.ev[0].Args["self"] != true {
		t.Fatalf("audit %+v", e.a.ev)
	}
	if code := e.del(b, "developer", pid, b); code != 404 {
		t.Fatalf("after leaving: %d", code)
	}
}

func TestT03_AC3_LastOwnerCannotLeave_HTTP(t *testing.T) {
	e := itSetup(t, true)
	a, b := uuid.NewString(), uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	if code := e.del(a, "developer", pid, a); code != 409 {
		t.Fatalf("code %d", code)
	}
	if _, o := e.count(t, pid); o != 1 || len(e.a.ev) != 0 {
		t.Fatalf("owners %d audit %d", o, len(e.a.ev))
	}
}

func TestT03_AC3_SuperAdminCanRemoveLastOwner(t *testing.T) {
	e := itSetup(t, true)
	a := uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner"})
	if code := e.del(uuid.NewString(), "super_admin", pid, a); code != 204 {
		t.Fatalf("code %d", code)
	}
	var n int
	if err := e.d.GetContext(context.Background(), &n, `SELECT count(*) FROM projects WHERE id=$1`, pid); err != nil || n != 1 {
		t.Fatalf("project gone: %d %v", n, err)
	}
	if m, _ := e.count(t, pid); m != 0 || len(e.a.ev) != 1 || e.a.ev[0].Args["self"] != false {
		t.Fatalf("members %d audit %+v", m, e.a.ev)
	}
}

func TestT03_AC3_ConcurrentOwnersLeave(t *testing.T) {
	e := itSetup(t, true)
	for i := 0; i < 20; i++ {
		a, c := uuid.NewString(), uuid.NewString()
		pid := e.project(t, map[string]string{a: "owner", c: "owner"})
		codes := make([]int, 2)
		var wg sync.WaitGroup
		for j, id := range []string{a, c} {
			wg.Add(1)
			go func() { defer wg.Done(); codes[j] = e.del(id, "developer", pid, id) }()
		}
		wg.Wait()
		if !(codes[0] == 204 && codes[1] == 409) && !(codes[0] == 409 && codes[1] == 204) {
			t.Fatalf("round %d: %v", i, codes)
		}
		if _, o := e.count(t, pid); o != 1 {
			t.Fatalf("round %d owners %d", i, o)
		}
	}
}

func TestRemoveHTTP_TargetNotMember404(t *testing.T) {
	e := itSetup(t, true)
	a := uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner"})
	if code := e.del(a, "developer", pid, uuid.NewString()); code != 404 {
		t.Fatalf("uuid: %d", code)
	}
	if code := e.del(a, "developer", pid, "bukan-uuid"); code != 404 {
		t.Fatalf("non-uuid: %d", code)
	}
	if len(e.a.ev) != 0 {
		t.Fatal("audit emitted")
	}
}

func TestT03_Security_NonMemberCannotRemove(t *testing.T) {
	e := itSetup(t, true)
	a, b := uuid.NewString(), uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	x := uuid.NewString()
	for _, p := range []string{pid, uuid.NewString(), "bukan-uuid"} {
		for _, tg := range []string{b, uuid.NewString(), "bukan-uuid"} {
			if code := e.del(x, "developer", p, tg); code != 404 {
				t.Fatalf("%s/%s: %d", p, tg, code)
			}
		}
	}
	if m, _ := e.count(t, pid); m != 2 {
		t.Fatalf("members %d", m)
	}
}

func TestRemoveHTTP_GetUserFailSoft(t *testing.T) {
	e := itSetup(t, false)
	a, b := uuid.NewString(), uuid.NewString()
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	if code := e.del(a, "developer", pid, b); code != 204 {
		t.Fatalf("code %d", code)
	}
	if len(e.a.ev) != 1 || e.a.ev[0].Args["user_label"] != "" {
		t.Fatalf("audit %+v", e.a.ev)
	}
}
