//go:build integration

package revokeinvitation

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

func itSetup(t *testing.T, registered bool, pendingIDs map[string]bool) *itEnv {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatal(err)
	}
	b := bus.NewRegistry(nil)
	if registered {
		b.Register(constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
			id := req.(getuser.Request).UserID
			resp := getuser.Response{Found: true, ID: id, Email: id[:4] + "@tim.id", Role: "developer", IsActive: true}
			if pendingIDs[id] {
				resp.Invitation = &getuser.Invitation{}
			}
			return resp, nil
		})
	}
	e := &itEnv{d: d, a: &itAudit{}}
	uc := NewUsecase(newRepo(d, store.New(d)), users.New(b), e.a, slog.Default())
	r := chi.NewRouter()
	r.Delete("/projects/{projectID}/invitations/{userID}", handler{uc: uc}.ServeHTTP)
	e.h = r
	return e
}

func (e *itEnv) del(uid, role, pid, target string) int {
	req := httptest.NewRequest(http.MethodDelete, "/projects/"+pid+"/invitations/"+target, nil)
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

func TestT12_AC8_OwnerRevokesPending(t *testing.T) {
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, map[string]bool{b: true})
	pid := e.project(t, map[string]string{a: "owner", b: "member", c: "member"})
	if code := e.del(a, "developer", pid, b); code != 204 {
		t.Fatalf("code %d", code)
	}
	if m, _ := e.count(t, pid); m != 2 {
		t.Fatalf("members %d", m)
	}
	if len(e.a.ev) != 1 || e.a.ev[0].Action != "project.invitation.revoked" {
		t.Fatalf("audit %+v", e.a.ev)
	}
	if code := e.del(a, "developer", pid, b); code != 404 {
		t.Fatalf("ulang: %d", code)
	}
	if len(e.a.ev) != 1 {
		t.Fatal("audit baru pada ulang")
	}
}

func TestT12_AC5_MemberForbidden(t *testing.T) {
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, map[string]bool{c: true})
	pid := e.project(t, map[string]string{a: "owner", b: "member", c: "member"})
	if code := e.del(b, "developer", pid, c); code != 403 {
		t.Fatalf("code %d", code)
	}
	if m, _ := e.count(t, pid); m != 3 || len(e.a.ev) != 0 {
		t.Fatalf("members %d audit %d", m, len(e.a.ev))
	}
}

func TestT12_AC5_NonMemberNotFound(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, map[string]bool{b: true})
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	for _, p := range []string{pid, uuid.NewString(), "bukan-uuid"} {
		if code := e.del(uuid.NewString(), "developer", p, b); code != 404 {
			t.Fatalf("%s: %d", p, code)
		}
	}
	if m, _ := e.count(t, pid); m != 2 {
		t.Fatalf("members %d", m)
	}
}

func TestRevokeHTTP_SuperAdmin(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, map[string]bool{b: true})
	pid := e.project(t, map[string]string{a: "owner", b: "owner"})
	if code := e.del(uuid.NewString(), "super_admin", pid, b); code != 204 {
		t.Fatalf("code %d", code)
	}
}

func TestRevokeHTTP_AlreadyAccepted409(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, nil)
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	if code := e.del(a, "developer", pid, b); code != 409 {
		t.Fatalf("code %d", code)
	}
	if m, _ := e.count(t, pid); m != 2 || len(e.a.ev) != 0 {
		t.Fatalf("members %d", m)
	}
}

func TestRevokeHTTP_PendingButNotMember404(t *testing.T) {
	a, x := uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, map[string]bool{x: true})
	pid := e.project(t, map[string]string{a: "owner"})
	if code := e.del(a, "developer", pid, x); code != 404 {
		t.Fatalf("code %d", code)
	}
	if code := e.del(a, "developer", pid, "bukan-uuid"); code != 404 {
		t.Fatalf("non-uuid %d", code)
	}
}

func TestRevokeHTTP_BusNotRegistered500(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	e := itSetup(t, false, nil)
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	if code := e.del(a, "developer", pid, b); code != 500 {
		t.Fatalf("code %d", code)
	}
	if m, _ := e.count(t, pid); m != 2 || len(e.a.ev) != 0 {
		t.Fatal("efek samping")
	}
}

func TestRevokeHTTP_ConcurrentSameTarget(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	e := itSetup(t, true, map[string]bool{b: true})
	pid := e.project(t, map[string]string{a: "owner", b: "member"})
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func() { defer wg.Done(); codes[i] = e.del(a, "developer", pid, b) }()
	}
	wg.Wait()
	if !(codes[0] == 204 && codes[1] == 404) && !(codes[0] == 404 && codes[1] == 204) {
		t.Fatalf("%v", codes)
	}
	if len(e.a.ev) != 1 {
		t.Fatalf("audit %d", len(e.a.ev))
	}
}
