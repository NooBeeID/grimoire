package buslistchanges

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

const pid = "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"

type fakeRepo struct {
	items     []listchanges.Item
	nilItems  bool
	total     int
	err       error
	calls     int
	lastLike  string
	lastLimit int
	lastSince time.Time
}

func (f *fakeRepo) ListChanges(_ context.Context, _ string, since time.Time, like string, limit int) ([]listchanges.Item, int, error) {
	f.calls++
	f.lastLike, f.lastLimit, f.lastSince = like, limit, since
	if f.nilItems {
		return nil, 0, f.err
	}
	return f.items, f.total, f.err
}

func since() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }

func TestExecute_ForwardsAndClampsLimit(t *testing.T) {
	for _, c := range []struct{ in, want int }{{0, 20}, {-3, 20}, {7, 7}, {999, 50}} {
		f := &fakeRepo{}
		_, err := New(f).Execute(context.Background(), listchanges.Request{ProjectID: pid, Since: since(), Prefix: "/docs", Limit: c.in})
		if err != nil {
			t.Fatal(err)
		}
		if f.lastLimit != c.want {
			t.Errorf("limit %d -> %d, want %d", c.in, f.lastLimit, c.want)
		}
		if f.lastLike != "docs/%" || !f.lastSince.Equal(since()) {
			t.Errorf("like=%q since=%v", f.lastLike, f.lastSince)
		}
	}
}

func TestExecute_InvalidInputs(t *testing.T) {
	cases := []listchanges.Request{
		{ProjectID: "x", Since: since()},
		{ProjectID: pid},
		{ProjectID: pid, Since: since(), Prefix: "a/../b"},
	}
	for i, req := range cases {
		f := &fakeRepo{}
		_, err := New(f).Execute(context.Background(), req)
		if !errors.Is(err, errs.ErrInvalid) {
			t.Errorf("case %d: err = %v", i, err)
		}
		if f.calls != 0 {
			t.Errorf("case %d: repo dipanggil", i)
		}
	}
}

func TestExecute_EmptyItemsNotNil(t *testing.T) {
	res, err := New(&fakeRepo{nilItems: true}).Execute(context.Background(), listchanges.Request{ProjectID: pid, Since: since()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items == nil || len(res.Items) != 0 || res.Total != 0 {
		t.Fatalf("%#v", res)
	}
}

func TestExecute_PropagatesRepoError(t *testing.T) {
	boom := errors.New("boom")
	_, err := New(&fakeRepo{err: boom}).Execute(context.Background(), listchanges.Request{ProjectID: pid, Since: since()})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func newReg(repo Repository) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := &Handler{uc: New(repo)}
	reg.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES, h.Handle)
	return reg
}

func TestHandle_WrongRequestType(t *testing.T) {
	f := &fakeRepo{}
	reg := newReg(f)
	for _, req := range []any{&listchanges.Request{}, "s", nil} {
		_, err := bus.CallTyped[listchanges.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestHandle_Success(t *testing.T) {
	f := &fakeRepo{items: []listchanges.Item{{Path: "a.md", SizeBytes: 3}}, total: 1}
	res, err := bus.CallTyped[listchanges.Response](context.Background(), newReg(f), constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES,
		listchanges.Request{ProjectID: pid, Since: since()})
	if err != nil || len(res.Items) != 1 || res.Total != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}
