package deletegroup

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	return base.Actor{ID: "u", Label: "Budi"}, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

// fakeTx menyimpan satu grup; Delete kedua kali tidak menemukan apa-apa.
type fakeTx struct{ group *base.Group }

func (f *fakeTx) LockProject(context.Context, string) error { return nil }
func (f *fakeTx) Delete(context.Context, string, string) (base.Group, bool, error) {
	if f.group == nil {
		return base.Group{}, false, nil
	}
	g := *f.group
	f.group = nil
	return g, true, nil
}

type fakeRepo struct {
	tx    *fakeTx
	calls int
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error { f.calls++; return fn(f.tx) }

func TestDelete_TwiceBothReported(t *testing.T) {
	tx := &fakeTx{group: &base.Group{ID: "g1", Terms: []string{"a", "b"}}}
	r, a := &fakeRepo{tx: tx}, &fakeAudit{}
	uc := NewUsecase(fakeAccess{}, r, a)

	if err := uc.Execute(context.Background(), "p", "g1", base.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if len(a.events) != 1 || a.events[0].Action != record.ActionSearchSynonymDeleted || a.events[0].Target != "g1" ||
		a.events[0].Result["terms_count"] != 2 {
		t.Fatalf("audit = %+v", a.events)
	}
	// Kedua: 404, audit tidak bertambah.
	if err := uc.Execute(context.Background(), "p", "g1", base.RequestMeta{}); !errors.Is(err, base.ErrGroupNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(a.events) != 1 {
		t.Fatalf("audit = %d", len(a.events))
	}
}

func TestDelete_NotMember(t *testing.T) {
	r, a := &fakeRepo{tx: &fakeTx{group: &base.Group{ID: "g1"}}}, &fakeAudit{}
	err := NewUsecase(fakeAccess{err: base.ErrProjectNotFound}, r, a).Execute(context.Background(), "p", "g1", base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) || r.calls != 0 || len(a.events) != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestDelete_RepoError(t *testing.T) {
	boom := errors.New("boom")
	r, a := &fakeRepo{tx: &fakeTx{}}, &fakeAudit{}
	uc := NewUsecase(fakeAccess{}, &errRepo{err: boom}, a)
	if err := uc.Execute(context.Background(), "p", "g1", base.RequestMeta{}); !errors.Is(err, boom) || len(a.events) != 0 {
		t.Fatalf("err = %v", err)
	}
	_ = r
}

type errRepo struct{ err error }

func (e *errRepo) InTx(context.Context, func(Tx) error) error { return e.err }
