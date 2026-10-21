//go:build integration

package checkaccess

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base/store"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/project/management/checkaccess"
)

type authFake struct {
	mu       sync.Mutex
	inactive map[string]bool
	calls    map[string]int
}

func (a *authFake) handle(_ context.Context, r any) (any, error) {
	req := r.(getuser.Request)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[req.UserID]++
	return getuser.Response{Found: true, ID: req.UserID, Role: "developer", IsActive: !a.inactive[req.UserID]}, nil
}

type env struct {
	b                  *bus.Registry
	d                  *db.DB
	auth               *authFake
	pid, A, B, C, D, S string
}

func setup(t *testing.T, withAuth bool) *env {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	e := &env{d: d, b: bus.NewRegistry(nil), auth: &authFake{inactive: map[string]bool{}, calls: map[string]int{}},
		A: uuid.NewString(), B: uuid.NewString(), C: uuid.NewString(), D: uuid.NewString(), S: uuid.NewString()}
	if withAuth {
		e.b.Register(constants.BUS_CORE_AUTH_GET_USER, e.auth.handle)
	}
	uc := NewUsecase(store.New(d), users.New(e.b))
	e.b.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, newHandler(uc).Handle)
	ctx := context.Background()
	if err := d.GetContext(ctx, &e.pid, `INSERT INTO projects (name) VALUES ('Project') RETURNING id::text`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,'owner'),($1,$3,'member'),($1,$4,'member')`,
		e.pid, e.A, e.B, e.D); err != nil {
		t.Fatal(err)
	}
	e.auth.inactive[e.D] = true
	return e
}

func (e *env) check(pid, uid, role string) (checkaccess.Response, error) {
	return bus.CallTyped[checkaccess.Response](context.Background(), e.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS,
		checkaccess.Request{ProjectID: pid, UserID: uid, UserRole: role})
}

func TestT03_AC5_DeactivatedMemberHasNoAccess(t *testing.T) {
	e := setup(t, true)
	if r, err := e.check(e.pid, e.B, "developer"); err != nil || r.Level != checkaccess.LevelMember {
		t.Fatalf("%+v %v", r, err)
	}
	e.auth.mu.Lock()
	e.auth.inactive[e.B] = true
	e.auth.mu.Unlock()
	if r, err := e.check(e.pid, e.B, "developer"); err != nil || r != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}) {
		t.Fatalf("%+v %v", r, err)
	}
	if r, _ := e.check(e.pid, e.D, "developer"); r.Level != checkaccess.LevelNone {
		t.Fatalf("D: %+v", r)
	}
	if r, _ := e.check(e.pid, e.A, "developer"); r.Level != checkaccess.LevelOwner {
		t.Fatalf("A: %+v", r)
	}
	if r, _ := e.check(e.pid, e.S, "super_admin"); r.Level != checkaccess.LevelAdmin || e.auth.calls[e.S] != 0 {
		t.Fatalf("S: %+v calls=%d", r, e.auth.calls[e.S])
	}
}

func TestT03_AC8_CheckAccessUnknownProject(t *testing.T) {
	e := setup(t, true)
	for _, pid := range []string{uuid.NewString(), "bukan-uuid"} {
		if r, err := e.check(pid, e.A, "developer"); err != nil || r != (checkaccess.Response{ProjectExists: false, Level: checkaccess.LevelNone}) {
			t.Fatalf("%s: %+v %v", pid, r, err)
		}
	}
	_, err := bus.CallTyped[checkaccess.Response](context.Background(), e.b, constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, "salah")
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("%v", err)
	}
	if _, err := e.check(e.pid, e.A, "agent"); !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("%v", err)
	}
}

func TestCheckAccess_NonMemberDeveloper(t *testing.T) {
	e := setup(t, true)
	if r, err := e.check(e.pid, e.C, "developer"); err != nil || r != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}) || e.auth.calls[e.C] != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestCheckAccess_FailClosedWhenGetUserUnregistered(t *testing.T) {
	e := setup(t, false)
	if _, err := e.check(e.pid, e.A, "developer"); !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("%v", err)
	}
	if r, err := e.check(e.pid, e.S, "super_admin"); err != nil || r.Level != checkaccess.LevelAdmin {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestCheckAccess_AfterMemberRemoved(t *testing.T) {
	e := setup(t, true)
	if r, _ := e.check(e.pid, e.B, "developer"); r.Level != checkaccess.LevelMember {
		t.Fatalf("%+v", r)
	}
	if _, err := e.d.ExecContext(context.Background(), `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, e.pid, e.B); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.check(e.pid, e.B, "developer"); r.Level != checkaccess.LevelNone {
		t.Fatalf("%+v", r)
	}
}

func TestCheckAccess_NoAuditCall(t *testing.T) {
	e := setup(t, true)
	audited := 0
	e.b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) { audited++; return nil, nil })
	_, _ = e.check(e.pid, e.A, "developer")
	_, _ = e.check(e.pid, e.S, "super_admin")
	if audited != 0 {
		t.Fatalf("audit dipanggil %d", audited)
	}
}
