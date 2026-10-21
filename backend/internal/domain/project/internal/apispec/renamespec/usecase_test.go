package renamespec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/domain/project/internal/apispec/base/audit"
	"grimoire/internal/domain/project/internal/apispec/base/store"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
)

const pid = "0b6c3e8a-6f0d-4a3b-9c1e-2d7f5a1b8c90"

var (
	fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	actor    = base.Actor{ID: "u1", Label: "Sari"}
)

type log struct{ calls []string }

type fakeGuard struct {
	l   *log
	err error
	n   int
}

func (g *fakeGuard) RequireMember(context.Context, string) error {
	g.n++
	g.l.calls = append(g.l.calls, "guard")
	return g.err
}

type fakeAudit struct {
	l      *log
	events []audit.Event
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	a.l.calls = append(a.l.calls, "audit")
	a.events = append(a.events, ev)
}

type fakeRepo struct {
	l                  *log
	spec               *base.Spec
	lockErr, renameErr error
	metaErr            error
	renamed            []base.Spec
	txs                int
}

func (r *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error {
	r.txs++
	err := fn(r)
	r.l.calls = append(r.l.calls, "commit")
	return err
}

func (r *fakeRepo) LockSpec(_ context.Context, _, name string) (base.Spec, bool, error) {
	r.l.calls = append(r.l.calls, "lock")
	if r.lockErr != nil {
		return base.Spec{}, false, r.lockErr
	}
	if r.spec == nil || r.spec.Name != name {
		return base.Spec{}, false, nil
	}
	return *r.spec, true, nil
}

func (r *fakeRepo) RenameSpec(_ context.Context, s base.Spec) error {
	r.l.calls = append(r.l.calls, "rename")
	if r.renameErr != nil {
		return r.renameErr
	}
	r.renamed = append(r.renamed, s)
	return nil
}

func (r *fakeRepo) FindSpecMeta(_ context.Context, _, name string) (base.SpecSummary, bool, error) {
	r.l.calls = append(r.l.calls, "meta")
	if r.metaErr != nil {
		return base.SpecSummary{}, false, r.metaErr
	}
	return base.SpecSummary{Name: name, EndpointCount: 2}, true, nil
}

type rig struct {
	l *log
	r *fakeRepo
	g *fakeGuard
	a *fakeAudit
	u *Usecase
}

func newRig() *rig {
	l := &log{}
	r := &fakeRepo{l: l, spec: &base.Spec{ID: "s1", ProjectID: pid, Name: "payment"}}
	g := &fakeGuard{l: l}
	a := &fakeAudit{l: l}
	return &rig{l: l, r: r, g: g, a: a, u: NewUsecase(r, g, a, func() time.Time { return fixedNow })}
}

func in(newName string) Input {
	return Input{ProjectID: pid, OldName: "payment", NewName: newName, Actor: actor, Meta: base.RequestMeta{IP: "1.1.1.1"}}
}

func TestExecute_Success(t *testing.T) {
	g := newRig()
	out, err := g.u.Execute(context.Background(), in("payment-v2"))
	if err != nil || out.Name != "payment-v2" || out.EndpointCount != 2 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(g.r.renamed) != 1 {
		t.Fatalf("renamed=%d", len(g.r.renamed))
	}
	s := g.r.renamed[0]
	if s.Name != "payment-v2" || s.UpdatedBy != "u1" || s.UpdatedByLabel != "Sari" || !s.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("spec=%+v", s)
	}
	if len(g.a.events) != 1 {
		t.Fatalf("events=%d", len(g.a.events))
	}
	ev := g.a.events[0]
	if ev.Action != record.ActionAPISpecUpdated || ev.Target != "payment-v2" || ev.ProjectID != pid ||
		!reflect.DeepEqual(ev.Args, map[string]any{"from": "payment", "to": "payment-v2"}) {
		t.Fatalf("event=%+v", ev)
	}
	want := []string{"guard", "lock", "rename", "meta", "commit", "audit"}
	if !reflect.DeepEqual(g.l.calls, want) {
		t.Fatalf("calls=%v want %v", g.l.calls, want)
	}
}

func TestExecute_InvalidName(t *testing.T) {
	for _, n := range []string{"Bad Name", "", strings.Repeat("a", 81), "a/b", "..", "Payment"} {
		g := newRig()
		_, err := g.u.Execute(context.Background(), in(n))
		if !errors.Is(err, base.ErrInvalidName) {
			t.Fatalf("%q: err=%v", n, err)
		}
		if len(g.l.calls) != 0 {
			t.Fatalf("%q: calls=%v", n, g.l.calls)
		}
	}
}

func TestExecute_NonMember(t *testing.T) {
	g := newRig()
	g.g.err = base.ErrProjectNotFound
	_, err := g.u.Execute(context.Background(), in("x"))
	if !errors.Is(err, base.ErrProjectNotFound) || g.r.txs != 0 || len(g.a.events) != 0 {
		t.Fatalf("err=%v txs=%d", err, g.r.txs)
	}
}

func TestExecute_GuardInfraError(t *testing.T) {
	g := newRig()
	boom := errors.New("bus down")
	g.g.err = boom
	_, err := g.u.Execute(context.Background(), in("x"))
	if !errors.Is(err, boom) || g.r.txs != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_SpecNotFound(t *testing.T) {
	g := newRig()
	g.r.spec = nil
	_, err := g.u.Execute(context.Background(), in("x"))
	if !errors.Is(err, base.ErrSpecNotFound) || len(g.r.renamed) != 0 || len(g.a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_SameNameNoop(t *testing.T) {
	g := newRig()
	out, err := g.u.Execute(context.Background(), in("payment"))
	if err != nil || out.Name != "payment" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(g.r.renamed) != 0 || len(g.a.events) != 0 {
		t.Fatalf("renamed=%d events=%d", len(g.r.renamed), len(g.a.events))
	}
}

func TestExecute_NameTaken(t *testing.T) {
	g := newRig()
	g.r.renameErr = store.ErrNameTaken
	_, err := g.u.Execute(context.Background(), in("other"))
	if !errors.Is(err, base.ErrNameTaken) || len(g.a.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_InfraErrors(t *testing.T) {
	boom := errors.New("db down")
	cases := map[string]func(*fakeRepo){
		"lock":   func(r *fakeRepo) { r.lockErr = boom },
		"rename": func(r *fakeRepo) { r.renameErr = boom },
		"meta":   func(r *fakeRepo) { r.metaErr = boom },
	}
	for name, mod := range cases {
		g := newRig()
		mod(g.r)
		_, err := g.u.Execute(context.Background(), in("payment-v2"))
		var e *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &e) || len(g.a.events) != 0 {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}
