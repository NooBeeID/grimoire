package creategroup

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/domain/project/internal/searchsynonym/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
)

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	return base.Actor{ID: "u1", Label: "Budi"}, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

type fakeTx struct {
	count    int
	used     []string
	insertEr error
	locked   int
	inserted []base.Group
}

func (f *fakeTx) LockProject(context.Context, string) error  { f.locked++; return nil }
func (f *fakeTx) Count(context.Context, string) (int, error) { return f.count, nil }
func (f *fakeTx) TermsInUse(context.Context, string, []string, string) ([]string, error) {
	return f.used, nil
}
func (f *fakeTx) Insert(_ context.Context, g base.Group) (base.Group, error) {
	if f.insertEr != nil {
		return base.Group{}, f.insertEr
	}
	g.ID = "g1"
	f.inserted = append(f.inserted, g)
	return g, nil
}

type fakeRepo struct {
	tx    *fakeTx
	calls int
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error {
	f.calls++
	return fn(f.tx)
}

func setup(acc error, tx *fakeTx) (*Usecase, *fakeRepo, *fakeAudit) {
	r, a := &fakeRepo{tx: tx}, &fakeAudit{}
	return NewUsecase(fakeAccess{err: acc}, r, a), r, a
}

func detailsOf(t *testing.T, err error) map[string]any {
	t.Helper()
	var ee *errs.Error
	if !errors.As(err, &ee) {
		t.Fatalf("bukan errs.Error: %v", err)
	}
	m, _ := ee.Details.(map[string]any)
	return m
}

func TestCreate_OK(t *testing.T) {
	tx := &fakeTx{}
	uc, _, a := setup(nil, tx)
	dto, err := uc.Execute(context.Background(), "p", Input{Terms: []string{" Login ", "Masuk"}}, base.RequestMeta{IP: "1.1.1.1"})
	if err != nil || dto.ID != "g1" || !reflect.DeepEqual(dto.Terms, []string{"login", "masuk"}) || dto.CreatedByLabel != "Budi" {
		t.Fatalf("%+v %v", dto, err)
	}
	if tx.locked != 1 {
		t.Fatalf("lock = %d", tx.locked)
	}
	if len(a.events) != 1 {
		t.Fatalf("audit = %d", len(a.events))
	}
	ev := a.events[0]
	if ev.Action != record.ActionSearchSynonymCreated || ev.Target != "g1" || ev.TargetType != audit.TargetSynonymGroup ||
		ev.Result["terms_count"] != 2 || ev.Meta.IP != "1.1.1.1" {
		t.Fatalf("%+v", ev)
	}
}

func TestCreate_InvalidDoesNotTouchRepo(t *testing.T) {
	uc, r, a := setup(nil, &fakeTx{})
	_, err := uc.Execute(context.Background(), "p", Input{Terms: []string{"a"}}, base.RequestMeta{})
	if !errors.Is(err, errs.ErrInvalid) || detailsOf(t, err)["reason"] != base.ReasonTooFewTerms {
		t.Fatalf("err = %v", err)
	}
	if r.calls != 0 || len(a.events) != 0 {
		t.Fatalf("repo=%d audit=%d", r.calls, len(a.events))
	}
}

func TestCreate_TermInUse(t *testing.T) {
	tx := &fakeTx{used: []string{"login"}}
	uc, _, a := setup(nil, tx)
	_, err := uc.Execute(context.Background(), "p", Input{Terms: []string{"login", "masuk"}}, base.RequestMeta{})
	d := detailsOf(t, err)
	if d["reason"] != base.ReasonTermInUse || !reflect.DeepEqual(d["terms"], []string{"login"}) {
		t.Fatalf("details = %v err=%v", d, err)
	}
	if len(tx.inserted) != 0 || len(a.events) != 0 {
		t.Fatal("tidak boleh insert/audit")
	}
}

func TestCreate_GroupLimit(t *testing.T) {
	uc, _, a := setup(nil, &fakeTx{count: base.MaxGroupsPerProject})
	_, err := uc.Execute(context.Background(), "p", Input{Terms: []string{"a", "b"}}, base.RequestMeta{})
	if detailsOf(t, err)["reason"] != base.ReasonGroupLimit || len(a.events) != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestCreate_NotMember(t *testing.T) {
	uc, r, a := setup(base.ErrProjectNotFound, &fakeTx{})
	_, err := uc.Execute(context.Background(), "p", Input{Terms: []string{"a", "b"}}, base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) || r.calls != 0 || len(a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestCreate_InsertErrorNoAudit(t *testing.T) {
	boom := errors.New("boom")
	uc, _, a := setup(nil, &fakeTx{insertEr: boom})
	_, err := uc.Execute(context.Background(), "p", Input{Terms: []string{"a", "b"}}, base.RequestMeta{})
	if !errors.Is(err, boom) || len(a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}
