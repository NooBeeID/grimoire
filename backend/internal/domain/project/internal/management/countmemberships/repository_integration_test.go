//go:build integration

package countmemberships

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/countmemberships"
)

type env struct {
	d *db.DB
	b *bus.Registry
}

func setup(t *testing.T) env {
	t.Helper()
	d := db.TestDB(t)
	if _, err := db.Up(context.Background(), d, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{DB: d, Bus: b})
	return env{d: d, b: b}
}

func (e env) project(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1 || ' project') RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) member(t *testing.T, pid, uid, role string) {
	t.Helper()
	if _, err := e.d.ExecContext(context.Background(), `INSERT INTO project_members (project_id, user_id, role) VALUES ($1,$2,$3)`, pid, uid, role); err != nil {
		t.Fatal(err)
	}
}

func (e env) count(ids ...string) (countmemberships.Response, error) {
	return bus.CallTyped[countmemberships.Response](context.Background(), e.b, constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS,
		countmemberships.Request{UserIDs: ids})
}

func TestCountMemberships_AcrossProjects(t *testing.T) {
	e := setup(t)
	p1, p2, p3 := e.project(t, "A"), e.project(t, "B"), e.project(t, "C")
	a, b, c, d := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.member(t, p1, a, "owner")
	e.member(t, p2, a, "member")
	e.member(t, p3, a, "member")
	e.member(t, p1, b, "member")
	e.member(t, p1, c, "member")
	e.member(t, p2, c, "member")
	e.member(t, p1, d, "member")
	res, err := e.count(a, b, c)
	if err != nil || len(res.Counts) != 3 || res.Counts[a] != 3 || res.Counts[b] != 1 || res.Counts[c] != 2 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if _, ok := res.Counts[d]; ok {
		t.Fatal("user lain tidak boleh muncul")
	}
	if _, err := e.d.ExecContext(context.Background(), `DELETE FROM project_members WHERE user_id=$1 AND project_id=$2`, a, p3); err != nil {
		t.Fatal(err)
	}
	if res, err = e.count(a); err != nil || res.Counts[a] != 2 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountMemberships_AbsentAndInvalid(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	a, d := uuid.NewString(), uuid.NewString()
	e.member(t, p1, a, "owner")
	res, err := e.count(a, d, "bukan-uuid")
	if err != nil || len(res.Counts) != 1 || res.Counts[a] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if _, ok := res.Counts[d]; ok {
		t.Fatal("d harus absen")
	}
	res, err = e.count(d)
	if err != nil || res.Counts == nil || len(res.Counts) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountMemberships_UppercaseCanonicalKey(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	a := uuid.NewString()
	e.member(t, p1, a, "owner")
	res, err := e.count(strings.ToUpper(a), a)
	if err != nil || len(res.Counts) != 1 || res.Counts[a] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountMemberships_500IDsOK_501Invalid(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	a := uuid.NewString()
	e.member(t, p1, a, "owner")
	ids := []string{a}
	for len(ids) < 500 {
		ids = append(ids, uuid.NewString())
	}
	res, err := e.count(ids...)
	if err != nil || len(res.Counts) != 1 || res.Counts[a] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	ids = append(ids, uuid.NewString())
	if _, err := e.count(ids...); !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestCountMemberships_NoAuditNoAuthCalls(t *testing.T) {
	e := setup(t)
	calls := 0
	for _, ep := range []string{constants.BUS_CORE_AUDIT_RECORD, constants.BUS_CORE_AUTH_GET_USER} {
		e.b.Register(ep, func(context.Context, any) (any, error) { calls++; return nil, nil })
	}
	p1 := e.project(t, "A")
	a := uuid.NewString()
	e.member(t, p1, a, "owner")
	res, err := e.count(a)
	if err != nil || res.Counts[a] != 1 || calls != 0 {
		t.Fatalf("res=%v err=%v calls=%d", res, err, calls)
	}
}
