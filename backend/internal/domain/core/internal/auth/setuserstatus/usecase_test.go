package setuserstatus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

type fakeTx struct {
	calls     []string
	adminIDs  []string
	user      base.User
	found     bool
	setErr    error
	delErr    error
	revoked   int64
	setActive []bool
}

func (f *fakeTx) LockActiveSuperAdminIDs(context.Context) ([]string, error) {
	f.calls = append(f.calls, "admins")
	return f.adminIDs, nil
}
func (f *fakeTx) LockUser(context.Context, string) (base.User, bool, error) {
	f.calls = append(f.calls, "lock")
	return f.user, f.found, nil
}
func (f *fakeTx) SetActive(_ context.Context, _ string, a bool, _ time.Time) error {
	f.calls = append(f.calls, "set")
	f.setActive = append(f.setActive, a)
	return f.setErr
}
func (f *fakeTx) DeleteSessions(context.Context, string) (int64, error) {
	f.calls = append(f.calls, "del")
	return f.revoked, f.delErr
}

type fakeRepo struct{ tx *fakeTx }

func (r fakeRepo) InTx(_ context.Context, fn func(Tx) error) error { return fn(r.tx) }

type fakeRec struct{ evs []audit.Event }

func (f *fakeRec) Record(_ context.Context, ev audit.Event) { f.evs = append(f.evs, ev) }

var t0 = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

func setup(tx *fakeTx) (*Usecase, *fakeRec) {
	rec := &fakeRec{}
	return NewUsecase(fakeRepo{tx}, rec, func() time.Time { return t0 }), rec
}

func actorCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "admin-1", Email: "a@x.id", Role: base.RoleSuperAdmin})
}

func devUser(active bool) base.User {
	return base.User{ID: "dev-1", Email: "d@x.id", Role: base.RoleDeveloper, IsActive: active}
}

func reason(t *testing.T, err error) (errs.Code, any) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	d, _ := e.Details.(map[string]any)
	return e.Code, d["reason"]
}

func TestExecute_Deactivate_Success(t *testing.T) {
	tx := &fakeTx{adminIDs: []string{"admin-1", "admin-2"}, user: devUser(true), found: true, revoked: 3}
	uc, rec := setup(tx)
	dto, err := uc.Execute(actorCtx(), Input{TargetID: "dev-1", Active: false})
	if err != nil || dto.IsActive || !dto.UpdatedAt.Equal(t0) {
		t.Fatalf("%+v %v", dto, err)
	}
	if got := strings.Join(tx.calls, ","); got != "admins,lock,set,del" {
		t.Fatalf("urutan: %s", got)
	}
	if len(rec.evs) != 1 || rec.evs[0].Action != "user.deactivated" || rec.evs[0].Args["revoked_sessions"] != int64(3) || rec.evs[0].Target != "dev-1" {
		t.Fatalf("%+v", rec.evs)
	}
}

func TestExecute_Activate_Success(t *testing.T) {
	tx := &fakeTx{user: devUser(false), found: true}
	uc, rec := setup(tx)
	dto, err := uc.Execute(actorCtx(), Input{TargetID: "dev-1", Active: true})
	if err != nil || !dto.IsActive {
		t.Fatalf("%+v %v", dto, err)
	}
	if got := strings.Join(tx.calls, ","); got != "lock,set" {
		t.Fatalf("urutan: %s", got)
	}
	if len(rec.evs) != 1 || rec.evs[0].Action != "user.activated" || len(rec.evs[0].Args) != 0 {
		t.Fatalf("%+v", rec.evs)
	}
}

func TestExecute_Deactivate_Self(t *testing.T) {
	u := base.User{ID: "admin-1", Role: base.RoleSuperAdmin, IsActive: true}
	tx := &fakeTx{adminIDs: []string{"admin-1", "admin-2"}, user: u, found: true}
	uc, rec := setup(tx)
	_, err := uc.Execute(actorCtx(), Input{TargetID: "admin-1"})
	code, r := reason(t, err)
	if code != errs.CodeForbidden || r != base.ReasonSelfAction || len(tx.setActive) != 0 || len(rec.evs) != 0 {
		t.Fatalf("%v %v", code, r)
	}
}

func TestExecute_Deactivate_LastSuperAdmin(t *testing.T) {
	u := base.User{ID: "admin-2", Role: base.RoleSuperAdmin, IsActive: true}
	tx := &fakeTx{adminIDs: []string{"admin-2"}, user: u, found: true}
	uc, rec := setup(tx)
	_, err := uc.Execute(actorCtx(), Input{TargetID: "admin-2"})
	code, r := reason(t, err)
	if code != errs.CodeConflict || r != base.ReasonLastSuperAdmin || len(rec.evs) != 0 || len(tx.setActive) != 0 {
		t.Fatalf("%v %v", code, r)
	}
}

func TestExecute_NoOp(t *testing.T) {
	for _, active := range []bool{false, true} {
		tx := &fakeTx{adminIDs: []string{"admin-1"}, user: devUser(active), found: true}
		uc, rec := setup(tx)
		dto, err := uc.Execute(actorCtx(), Input{TargetID: "dev-1", Active: active})
		if err != nil || dto.IsActive != active {
			t.Fatalf("active=%v %+v %v", active, dto, err)
		}
		if len(tx.setActive) != 0 || len(rec.evs) != 0 || contains(tx.calls, "del") {
			t.Fatalf("active=%v calls=%v", active, tx.calls)
		}
		if !dto.UpdatedAt.IsZero() {
			t.Fatalf("updated_at tersentuh: %v", dto.UpdatedAt)
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestExecute_TargetNotFound(t *testing.T) {
	tx := &fakeTx{adminIDs: []string{"admin-1"}}
	uc, rec := setup(tx)
	_, err := uc.Execute(actorCtx(), Input{TargetID: "x"})
	if !errors.Is(err, base.ErrUserNotFound) || len(rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	uc, _ := setup(&fakeTx{})
	_, err := uc.Execute(context.Background(), Input{TargetID: "x"})
	if code, _ := reason(t, err); code != errs.CodeUnauthorized {
		t.Fatal(code)
	}
}

func TestExecute_InfraErrorWrapped(t *testing.T) {
	tx := &fakeTx{adminIDs: []string{"a", "b"}, user: devUser(true), found: true, setErr: errors.New("boom")}
	uc, rec := setup(tx)
	_, err := uc.Execute(actorCtx(), Input{TargetID: "dev-1"})
	if err == nil || !strings.Contains(err.Error(), "setuserstatus: transaksi:") || len(rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
	var e *errs.Error
	if errors.As(err, &e) {
		t.Fatal("error infra tidak boleh menjadi *errs.Error")
	}
}

func TestExecute_AuditNotCalledOnRollback(t *testing.T) {
	tx := &fakeTx{adminIDs: []string{"a", "b"}, user: devUser(true), found: true, delErr: errors.New("boom")}
	uc, rec := setup(tx)
	if _, err := uc.Execute(actorCtx(), Input{TargetID: "dev-1"}); err == nil || len(rec.evs) != 0 {
		t.Fatalf("%v", err)
	}
}
