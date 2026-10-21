package countkeys

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

func newBus() *bus.Registry {
	return bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func registerFake(f *fakeRepo) *bus.Registry {
	b := newBus()
	b.Register(constants.BUS_PROJECT_APIKEY_COUNT_KEYS, newHandler(NewUsecase(f)).Handle)
	return b
}

func TestHandle_WrongRequestType(t *testing.T) {
	f := &fakeRepo{}
	h := newHandler(NewUsecase(f))
	type other struct{ ProjectIDs []string }
	for _, req := range []any{&contentcount.Request{}, "x", nil, other{}} {
		res, err := h.Handle(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) || res != nil || f.calls != 0 {
			t.Fatalf("req=%T res=%v err=%v calls=%d", req, res, err, f.calls)
		}
	}
}

func TestHandle_ReturnsValueType(t *testing.T) {
	a := uuid.NewString()
	b := registerFake(&fakeRepo{result: map[string]int{a: 3}})
	res, err := bus.CallTyped[contentcount.Response](context.Background(), b, constants.BUS_PROJECT_APIKEY_COUNT_KEYS,
		contentcount.Request{ProjectIDs: []string{a}})
	if err != nil || res.Counts[a] != 3 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestHandle_ErrorReturnsNilResult(t *testing.T) {
	h := newHandler(NewUsecase(&fakeRepo{err: errors.New("boom")}))
	res, err := h.Handle(context.Background(), contentcount.Request{ProjectIDs: []string{uuid.NewString()}})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	b := newBus()
	Init(app.Deps{Bus: b})
	_, err := b.Call(context.Background(), constants.BUS_PROJECT_APIKEY_COUNT_KEYS, contentcount.Request{})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}
