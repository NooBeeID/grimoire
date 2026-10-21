package revokekey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

var (
	fixedNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	created  = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
)

type log struct{ events []string }

func (l *log) add(s string) { l.events = append(l.events, s) }

type fakeTx struct {
	l           *log
	key         base.Key
	found       bool
	lockErr     error
	markErr     error
	lockArgs    [2]string
	marked      *base.Key
	markedCalls int
}

func (f *fakeTx) LockInProject(_ context.Context, p, k string) (base.Key, bool, error) {
	f.l.add("lock")
	f.lockArgs = [2]string{p, k}
	return f.key, f.found, f.lockErr
}

func (f *fakeTx) MarkRevoked(_ context.Context, k base.Key) (base.Key, error) {
	f.l.add("mark")
	f.markedCalls++
	f.marked = &k
	if f.markErr != nil {
		return base.Key{}, f.markErr
	}
	return k, nil
}

type fakeRepo struct {
	l        *log
	tx       *fakeTx
	inTxErr  error // dikembalikan setelah fn sukses (simulasi commit gagal)
	inTxCall int
}

func (r *fakeRepo) InTx(_ context.Context, fn func(txRepo) error) error {
	r.inTxCall++
	r.l.add("begin")
	if err := fn(r.tx); err != nil {
		return err
	}
	if r.inTxErr != nil {
		return r.inTxErr
	}
	r.l.add("commit")
	return nil
}

type fakeAccess struct {
	l     *log
	err   error
	calls int
	pid   string
	actor base.Actor
}

func (a *fakeAccess) Require(_ context.Context, p string, act base.Actor) error {
	a.calls++
	a.pid, a.actor = p, act
	a.l.add("access")
	return a.err
}

type fakeAudit struct {
	l      *log
	events []audit.Event
}

func (a *fakeAudit) Record(_ context.Context, ev audit.Event) {
	a.l.add("audit")
	a.events = append(a.events, ev)
}

type rig struct {
	uc  *Usecase
	l   *log
	r   *fakeRepo
	tx  *fakeTx
	acc *fakeAccess
	aud *fakeAudit
}

func activeKey() base.Key {
	by, lbl := "creator", "c@x"
	return base.Key{ID: "k1", ProjectID: "p1", Name: "ci", Prefix: "abcd1234", SecretHash: []byte{1},
		CreatedBy: &by, CreatedByLabel: &lbl, CreatedAt: created}
}

func newRig() *rig {
	l := &log{}
	tx := &fakeTx{l: l, key: activeKey(), found: true}
	r := &fakeRepo{l: l, tx: tx}
	acc := &fakeAccess{l: l}
	aud := &fakeAudit{l: l}
	return &rig{uc: NewUsecase(r, acc, aud, func() time.Time { return fixedNow }), l: l, r: r, tx: tx, acc: acc, aud: aud}
}

func authed() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Email: "a@tim.id", Role: "developer"})
}

var meta = base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}

func in() Input { return Input{ProjectID: "p1", KeyID: "k1", Meta: meta} }

func TestExecute_Success(t *testing.T) {
	g := newRig()
	dto, err := g.uc.Execute(authed(), in())
	if err != nil {
		t.Fatal(err)
	}
	if g.acc.pid != "p1" || g.acc.actor != (base.Actor{ID: "u1", Role: "developer", Label: "a@tim.id"}) {
		t.Fatalf("access %+v %s", g.acc.actor, g.acc.pid)
	}
	if g.tx.lockArgs != [2]string{"p1", "k1"} {
		t.Fatalf("lock args %v", g.tx.lockArgs)
	}
	m := g.tx.marked
	if m == nil || m.RevokedAt == nil || !m.RevokedAt.Equal(fixedNow) || *m.RevokedBy != "u1" || *m.RevokedByLabel != "a@tim.id" {
		t.Fatalf("marked %+v", m)
	}
	if string(dto.Status) != "revoked" {
		t.Fatalf("status %v", dto.Status)
	}
	if len(g.aud.events) != 1 {
		t.Fatalf("audit %d", len(g.aud.events))
	}
	ev := g.aud.events[0]
	if ev.Action != record.ActionAPIKeyRevoked || ev.KeyID != "k1" || ev.ProjectID != "p1" || ev.Meta != meta ||
		len(ev.Args) != 2 || ev.Args["name"] != "ci" || ev.Args["prefix"] != "abcd1234" {
		t.Fatalf("event %+v", ev)
	}
	if got := strings.Join(g.l.events, ","); got != "access,begin,lock,mark,commit,audit" {
		t.Fatalf("order %s", got)
	}
}

func TestExecute_Unauthenticated(t *testing.T) {
	g := newRig()
	_, err := g.uc.Execute(context.Background(), in())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatalf("err=%v", err)
	}
	if len(g.l.events) != 0 {
		t.Fatalf("calls %v", g.l.events)
	}
}

func TestExecute_AccessErrors(t *testing.T) {
	for name, want := range map[string]error{"notfound": base.ErrProjectNotFound, "infra": errors.New("boom")} {
		t.Run(name, func(t *testing.T) {
			g := newRig()
			g.acc.err = want
			_, err := g.uc.Execute(authed(), in())
			if !errors.Is(err, want) {
				t.Fatalf("err=%v", err)
			}
			if g.r.inTxCall != 0 || len(g.aud.events) != 0 {
				t.Fatal("tx/audit dipanggil")
			}
		})
	}
}

func TestExecute_KeyNotFound(t *testing.T) {
	g := newRig()
	g.tx.found = false
	_, err := g.uc.Execute(authed(), in())
	if !errors.Is(err, base.ErrKeyNotFound) || g.tx.markedCalls != 0 || len(g.aud.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_AlreadyRevoked(t *testing.T) {
	g := newRig()
	at := created
	g.tx.key.RevokedAt = &at
	_, err := g.uc.Execute(authed(), in())
	if !errors.Is(err, base.ErrAlreadyRevoked) || g.tx.markedCalls != 0 || len(g.aud.events) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_InfraErrors(t *testing.T) {
	boom := errors.New("db down")
	cases := map[string]struct {
		setup func(*rig)
		want  string
	}{
		"lock":   {func(g *rig) { g.tx.lockErr = boom }, "revokekey: lock key"},
		"mark":   {func(g *rig) { g.tx.markErr = boom }, "revokekey: mark revoked"},
		"commit": {func(g *rig) { g.r.inTxErr = boom }, "revokekey: tx"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := newRig()
			c.setup(g)
			_, err := g.uc.Execute(authed(), in())
			var e *errs.Error
			if err == nil || errors.As(err, &e) || !errors.Is(err, boom) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err=%v", err)
			}
			if len(g.aud.events) != 0 {
				t.Fatal("audit dipanggil")
			}
		})
	}
}

func TestExecute_SentinelsUnchanged(t *testing.T) {
	nf, ar := *base.ErrKeyNotFound, *base.ErrAlreadyRevoked
	g := newRig()
	g.tx.found = false
	_, _ = g.uc.Execute(authed(), in())
	g = newRig()
	at := created
	g.tx.key.RevokedAt = &at
	_, _ = g.uc.Execute(authed(), in())
	if base.ErrKeyNotFound.Message != nf.Message || base.ErrAlreadyRevoked.Message != ar.Message ||
		base.ErrKeyNotFound.Code != nf.Code || base.ErrAlreadyRevoked.Code != ar.Code {
		t.Fatal("sentinel berubah")
	}
}
