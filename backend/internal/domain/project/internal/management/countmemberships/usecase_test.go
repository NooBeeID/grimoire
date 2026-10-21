package countmemberships

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/countmemberships"
)

type fakeRepo struct {
	calls  int
	got    []string
	result map[string]int
	err    error
}

func (f *fakeRepo) CountByUsers(_ context.Context, ids []string) (map[string]int, error) {
	f.calls++
	f.got = ids
	return f.result, f.err
}

func newBus() *bus.Registry {
	return bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCount_EmptyOrInvalid_NoQuery(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"bukan-uuid", "x"}} {
		f := &fakeRepo{}
		res, err := NewUsecase(f).Count(context.Background(), countmemberships.Request{UserIDs: in})
		if err != nil || res.Counts == nil || len(res.Counts) != 0 || f.calls != 0 {
			t.Fatalf("in=%v res=%v err=%v calls=%d", in, res, err, f.calls)
		}
	}
}

func TestCount_NormalizesAndDedupes(t *testing.T) {
	a := uuid.NewString()
	f := &fakeRepo{result: map[string]int{a: 2}}
	up := []rune(a)
	for i, r := range up {
		if r >= 'a' && r <= 'f' {
			up[i] = r - 32
		}
	}
	res, err := NewUsecase(f).Count(context.Background(), countmemberships.Request{UserIDs: []string{string(up), "bukan-uuid", a}})
	if err != nil || res.Counts[a] != 2 || !reflect.DeepEqual(f.got, []string{a}) {
		t.Fatalf("res=%v err=%v got=%v", res, err, f.got)
	}
}

func TestCount_Limit(t *testing.T) {
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = "x"
	}
	f := &fakeRepo{}
	_, err := NewUsecase(f).Count(context.Background(), countmemberships.Request{UserIDs: ids})
	if !errors.Is(err, errs.ErrInvalid) || f.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
	ok := make([]string, 500)
	for i := range ok {
		ok[i] = uuid.NewString()
	}
	if _, err := NewUsecase(f).Count(context.Background(), countmemberships.Request{UserIDs: ok}); err != nil || f.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
}

func TestCount_RepoError_Wrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewUsecase(&fakeRepo{err: boom}).Count(context.Background(), countmemberships.Request{UserIDs: []string{uuid.NewString()}})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

func TestCount_NilMapFromRepo_BecomesEmpty(t *testing.T) {
	res, err := NewUsecase(&fakeRepo{}).Count(context.Background(), countmemberships.Request{UserIDs: []string{uuid.NewString()}})
	if err != nil || res.Counts == nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestHandle_WrongRequestType(t *testing.T) {
	f := &fakeRepo{}
	h := newHandler(NewUsecase(f))
	for _, req := range []any{&countmemberships.Request{}, "x", nil} {
		res, err := h.Handle(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) || res != nil || f.calls != 0 {
			t.Fatalf("req=%T res=%v err=%v", req, res, err)
		}
	}
}

func TestHandle_ReturnsValueType_NoIdentity(t *testing.T) {
	a := uuid.NewString()
	b := newBus()
	b.Register(constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS, newHandler(NewUsecase(&fakeRepo{result: map[string]int{a: 3}})).Handle)
	res, err := bus.CallTyped[countmemberships.Response](context.Background(), b, constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS,
		countmemberships.Request{UserIDs: []string{a}})
	if err != nil || res.Counts[a] != 3 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestHandle_ErrorReturnsNilResult(t *testing.T) {
	h := newHandler(NewUsecase(&fakeRepo{err: errors.New("boom")}))
	res, err := h.Handle(context.Background(), countmemberships.Request{UserIDs: []string{uuid.NewString()}})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	b := newBus()
	Init(app.Deps{Bus: b})
	if _, err := b.Call(context.Background(), constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS, countmemberships.Request{}); err != nil {
		t.Fatalf("err=%v", err)
	}
}
