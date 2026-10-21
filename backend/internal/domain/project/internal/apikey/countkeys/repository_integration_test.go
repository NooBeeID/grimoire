//go:build integration

package countkeys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/db"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
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
	if err := e.d.GetContext(context.Background(), &id, `INSERT INTO projects (name) VALUES ($1) RETURNING id::text`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

var seq int

func (e env) key(t *testing.T, pid string, revoked bool) {
	t.Helper()
	seq++
	h := sha256.Sum256([]byte("s"))
	q := `INSERT INTO api_keys (project_id, name, prefix, secret_hash) VALUES ($1,$2,$3,$4)`
	if revoked {
		q = `INSERT INTO api_keys (project_id, name, prefix, secret_hash, revoked_at) VALUES ($1,$2,$3,$4, now())`
	}
	if _, err := e.d.ExecContext(context.Background(), q, pid, "k", fmt.Sprintf("pfx%05d", seq), h[:]); err != nil {
		t.Fatal(err)
	}
}

func (e env) count(ids ...string) (contentcount.Response, error) {
	return bus.CallTyped[contentcount.Response](context.Background(), e.b, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
		contentcount.Request{ProjectIDs: ids})
}

func TestCountKeys_OnlyActiveCounted(t *testing.T) {
	e := setup(t)
	p1, p2, p3 := e.project(t, "A"), e.project(t, "B"), e.project(t, "C")
	e.key(t, p1, false)
	e.key(t, p1, false)
	e.key(t, p1, true)
	e.key(t, p2, false)
	res, err := e.count(p1, p2, p3)
	if err != nil || res.Counts[p1] != 2 || res.Counts[p2] != 1 || res.Counts[p3] != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountKeys_OtherProjectsNotIncluded(t *testing.T) {
	e := setup(t)
	p1, p2 := e.project(t, "A"), e.project(t, "B")
	e.key(t, p1, false)
	e.key(t, p2, false)
	res, err := e.count(p1)
	if err != nil || len(res.Counts) != 1 || res.Counts[p1] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountKeys_AllRevoked_Absent(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	e.key(t, p1, true)
	res, err := e.count(p1)
	if err != nil || res.Counts[p1] != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountKeys_InvalidIDsIgnored(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	e.key(t, p1, false)
	e.key(t, p1, false)
	res, err := e.count(p1, "bukan-uuid")
	if err != nil || len(res.Counts) != 1 || res.Counts[p1] != 2 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountKeys_UppercaseUUID_KeyedCanonical(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	e.key(t, p1, false)
	res, err := e.count(strings.ToUpper(p1), p1)
	if err != nil || len(res.Counts) != 1 || res.Counts[p1] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountKeys_Over100_ErrInvalid(t *testing.T) {
	e := setup(t)
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "x"
	}
	if _, err := e.count(ids...); !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestCountKeys_WrongRequestType_ErrBadRequestType(t *testing.T) {
	e := setup(t)
	_, err := e.b.Call(context.Background(), constants.BUS_PROJECT_APIKEY_COUNT_KEYS, &contentcount.Request{})
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}

func TestCountKeys_100IDsAndSingleID(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	e.key(t, p1, false)
	ids := []string{p1}
	for i := 1; i < 100; i++ {
		ids = append(ids, e.project(t, fmt.Sprintf("P%d", i)))
	}
	res, err := e.count(ids...)
	if err != nil || res.Counts[p1] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if res, err = e.count(p1); err != nil || res.Counts[p1] != 1 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCountKeys_ProjectDeletedCascade(t *testing.T) {
	e := setup(t)
	p1 := e.project(t, "A")
	e.key(t, p1, false)
	if _, err := e.d.ExecContext(context.Background(), `DELETE FROM projects WHERE id = $1`, p1); err != nil {
		t.Fatal(err)
	}
	res, err := e.count(p1)
	if err != nil || res.Counts[p1] != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}
