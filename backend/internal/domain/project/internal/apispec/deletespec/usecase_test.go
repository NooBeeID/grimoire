package deletespec

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

const pid = "0b6c3e8a-6f0d-4a3b-9c1e-2d7f5a1b8c90"

type fakeGuard struct {
	err   error
	calls int
}

func (g *fakeGuard) RequireMember(context.Context, string) error { g.calls++; return g.err }

type fakeAudit struct {
	events []audit.Event
	repo   *fakeRepo
	inTx   []bool
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	a.events = append(a.events, ev)
	if a.repo != nil {
		a.inTx = append(a.inTx, a.repo.inTx)
	}
}

type fakeRepo struct {
	found      bool
	lockErr    error
	countErr   error
	deleteErr  error
	deleteNone bool
	txErr      error
	count      int
	calls      []string
	inTx       bool
	gotProject string
	gotName    string
}

func (r *fakeRepo) InTx(_ context.Context, fn func(txRepo) error) error {
	r.inTx = true
	defer func() { r.inTx = false }()
	if err := fn(r); err != nil {
		return err
	}
	return r.txErr
}

func (r *fakeRepo) LockSpec(_ context.Context, projectID, name string) (base.Spec, bool, error) {
	r.calls = append(r.calls, "lock")
	r.gotProject, r.gotName = projectID, name
	if r.lockErr != nil {
		return base.Spec{}, false, r.lockErr
	}
	return base.Spec{ID: "11111111-1111-1111-1111-111111111111", Name: name}, r.found, nil
}

func (r *fakeRepo) CountEndpoints(context.Context, string) (int, error) {
	r.calls = append(r.calls, "count")
	return r.count, r.countErr
}

func (r *fakeRepo) DeleteSpec(context.Context, string, string) (bool, error) {
	r.calls = append(r.calls, "delete")
	if r.deleteErr != nil {
		return false, r.deleteErr
	}
	return !r.deleteNone, nil
}

func authCtx() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Name: "Budi", Email: "b@tim.id"})
}

func setup(r *fakeRepo) (*Usecase, *fakeGuard, *fakeAudit) {
	g, a := &fakeGuard{}, &fakeAudit{repo: r}
	return NewUsecase(r, g, a), g, a
}

func TestExecute_Success(t *testing.T) {
	r := &fakeRepo{found: true, count: 12}
	uc, g, a := setup(r)
	meta := base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}
	if err := uc.Execute(authCtx(), Params{ProjectID: pid, Name: "payment"}, meta); err != nil {
		t.Fatal(err)
	}
	if g.calls != 1 || len(r.calls) != 3 || r.calls[0] != "lock" || r.calls[1] != "count" || r.calls[2] != "delete" {
		t.Fatalf("guard=%d calls=%v", g.calls, r.calls)
	}
	if r.gotProject != pid || r.gotName != "payment" {
		t.Fatalf("lock args %q %q", r.gotProject, r.gotName)
	}
	if len(a.events) != 1 {
		t.Fatalf("events=%d", len(a.events))
	}
	ev := a.events[0]
	if ev.Action != record.ActionAPISpecDeleted || ev.ProjectID != pid || ev.Target != "payment" || ev.Meta != meta {
		t.Fatalf("event=%+v", ev)
	}
	if ev.Args["name"] != "payment" || ev.Args["endpoint_count"] != 12 || len(ev.Args) != 2 {
		t.Fatalf("args=%v", ev.Args)
	}
	if a.inTx[0] {
		t.Fatal("audit dipanggil di dalam tx")
	}
}

func TestExecute_ZeroEndpoints(t *testing.T) {
	r := &fakeRepo{found: true}
	uc, _, a := setup(r)
	if err := uc.Execute(authCtx(), Params{ProjectID: pid, Name: "hooks"}, base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if a.events[0].Args["endpoint_count"] != 0 {
		t.Fatalf("args=%v", a.events[0].Args)
	}
}

func TestExecute_NoIdentity(t *testing.T) {
	r := &fakeRepo{found: true}
	uc, g, a := setup(r)
	err := uc.Execute(context.Background(), Params{ProjectID: pid, Name: "x"}, base.RequestMeta{})
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatalf("err=%v", err)
	}
	if g.calls != 0 || len(r.calls) != 0 || len(a.events) != 0 {
		t.Fatal("tidak boleh ada panggilan")
	}
}

func TestExecute_GuardErrors(t *testing.T) {
	for _, ge := range []error{base.ErrProjectNotFound, errors.New("bus down")} {
		r := &fakeRepo{found: true}
		uc, g, a := setup(r)
		g.err = ge
		err := uc.Execute(authCtx(), Params{ProjectID: pid, Name: "x"}, base.RequestMeta{})
		if !errors.Is(err, ge) {
			t.Fatalf("err=%v want %v", err, ge)
		}
		if len(r.calls) != 0 || len(a.events) != 0 {
			t.Fatal("repo/audit tidak boleh dipanggil")
		}
	}
}

func TestExecute_SpecNotFound(t *testing.T) {
	r := &fakeRepo{found: false}
	uc, _, a := setup(r)
	err := uc.Execute(authCtx(), Params{ProjectID: pid, Name: "x"}, base.RequestMeta{})
	if !errors.Is(err, base.ErrSpecNotFound) {
		t.Fatalf("err=%v", err)
	}
	if len(r.calls) != 1 || len(a.events) != 0 {
		t.Fatalf("calls=%v events=%d", r.calls, len(a.events))
	}
}

func TestExecute_InfraErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name      string
		r         *fakeRepo
		wantCalls int
	}{
		{"lock", &fakeRepo{found: true, lockErr: boom}, 1},
		{"count", &fakeRepo{found: true, countErr: boom}, 2},
		{"delete", &fakeRepo{found: true, deleteErr: boom}, 3},
		{"commit", &fakeRepo{found: true, txErr: boom}, 3},
	}
	for _, c := range cases {
		uc, _, a := setup(c.r)
		err := uc.Execute(authCtx(), Params{ProjectID: pid, Name: "x"}, base.RequestMeta{})
		if err == nil || !errors.Is(err, boom) {
			t.Fatalf("%s: err=%v", c.name, err)
		}
		var e *errs.Error
		if errors.As(err, &e) {
			t.Fatalf("%s: error infra tidak boleh *errs.Error", c.name)
		}
		if len(c.r.calls) != c.wantCalls || len(a.events) != 0 {
			t.Fatalf("%s: calls=%v events=%d", c.name, c.r.calls, len(a.events))
		}
	}
}

func TestExecute_DeleteNoRow(t *testing.T) {
	r := &fakeRepo{found: true, deleteNone: true}
	uc, _, a := setup(r)
	err := uc.Execute(authCtx(), Params{ProjectID: pid, Name: "x"}, base.RequestMeta{})
	if !errors.Is(err, base.ErrSpecNotFound) || len(a.events) != 0 {
		t.Fatalf("err=%v events=%d", err, len(a.events))
	}
}
