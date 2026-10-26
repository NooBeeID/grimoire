package updategroup

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

const gid = "0b6c0000-0000-4000-8000-0000000000bb"

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	return base.Actor{ID: "u2", Label: "Sari"}, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Record(_ context.Context, ev audit.Event) { f.events = append(f.events, ev) }

type fakeTx struct {
	existing   *base.Group
	used       []string
	exceptSeen string
	updated    []base.Group
}

func (f *fakeTx) LockProject(context.Context, string) error { return nil }
func (f *fakeTx) Get(context.Context, string, string) (base.Group, bool, error) {
	if f.existing == nil {
		return base.Group{}, false, nil
	}
	return *f.existing, true, nil
}
func (f *fakeTx) TermsInUse(_ context.Context, _ string, _ []string, except string) ([]string, error) {
	f.exceptSeen = except
	return f.used, nil
}
func (f *fakeTx) Update(_ context.Context, g base.Group) (base.Group, bool, error) {
	f.updated = append(f.updated, g)
	return g, true, nil
}

type fakeRepo struct {
	tx    *fakeTx
	calls int
}

func (f *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error { f.calls++; return fn(f.tx) }

func existing() *base.Group {
	return &base.Group{ID: gid, ProjectID: "p", Terms: []string{"login", "masuk"}, CreatedByLabel: "Budi", UpdatedByLabel: "Budi"}
}

func setup(acc error, tx *fakeTx) (*Usecase, *fakeRepo, *fakeAudit) {
	r, a := &fakeRepo{tx: tx}, &fakeAudit{}
	return NewUsecase(fakeAccess{err: acc}, r, a), r, a
}

func TestUpdate_ReplacesTermsAndActor(t *testing.T) {
	tx := &fakeTx{existing: existing()}
	uc, _, a := setup(nil, tx)
	// "login" milik grup ini sendiri: TermsInUse dipanggil dengan exceptID=gid (fake tidak melaporkannya).
	dto, err := uc.Execute(context.Background(), "p", gid, Input{Terms: []string{"Login", "Sign In", "Masuk"}}, base.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dto.Terms, []string{"login", "sign in", "masuk"}) || dto.UpdatedByLabel != "Sari" || dto.CreatedByLabel != "Budi" {
		t.Fatalf("%+v", dto)
	}
	if tx.exceptSeen != gid {
		t.Fatalf("exceptID = %q", tx.exceptSeen)
	}
	if len(a.events) != 1 || a.events[0].Action != record.ActionSearchSynonymUpdated || a.events[0].Target != gid ||
		a.events[0].Result["terms_count"] != 3 {
		t.Fatalf("audit = %+v", a.events)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	uc, _, a := setup(nil, &fakeTx{})
	_, err := uc.Execute(context.Background(), "p", gid, Input{Terms: []string{"a", "b"}}, base.RequestMeta{})
	if !errors.Is(err, base.ErrGroupNotFound) || len(a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestUpdate_TermInUse(t *testing.T) {
	tx := &fakeTx{existing: existing(), used: []string{"signin"}}
	uc, _, a := setup(nil, tx)
	_, err := uc.Execute(context.Background(), "p", gid, Input{Terms: []string{"login", "signin"}}, base.RequestMeta{})
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict || len(tx.updated) != 0 || len(a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestUpdate_InvalidInputsSkipRepo(t *testing.T) {
	cases := []struct {
		gid    string
		terms  []string
		reason base.Reason
	}{
		{"bukan-uuid", []string{"a", "b"}, base.ReasonInvalidGroupID},
		{gid, []string{"a"}, base.ReasonTooFewTerms},
		{gid, []string{"a&b", "c"}, base.ReasonTermInvalidChar},
	}
	for _, c := range cases {
		uc, r, a := setup(nil, &fakeTx{existing: existing()})
		_, err := uc.Execute(context.Background(), "p", c.gid, Input{Terms: c.terms}, base.RequestMeta{})
		var ee *errs.Error
		if !errors.As(err, &ee) || !errors.Is(err, errs.ErrInvalid) {
			t.Fatalf("%v: %v", c, err)
		}
		if m, _ := ee.Details.(map[string]any); m["reason"] != c.reason {
			t.Fatalf("%v: details %v", c, ee.Details)
		}
		if r.calls != 0 || len(a.events) != 0 {
			t.Fatalf("%v: repo=%d audit=%d", c, r.calls, len(a.events))
		}
	}
}

func TestUpdate_NotMember(t *testing.T) {
	uc, r, a := setup(base.ErrProjectNotFound, &fakeTx{existing: existing()})
	_, err := uc.Execute(context.Background(), "p", gid, Input{Terms: []string{"a", "b"}}, base.RequestMeta{})
	if !errors.Is(err, base.ErrProjectNotFound) || r.calls != 0 || len(a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}
