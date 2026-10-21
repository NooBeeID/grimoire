package countspecs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

func newReg(f *fakeCounter) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg.Register(constants.BUS_PROJECT_APISPEC_COUNT_SPECS, newHandler(NewUsecase(f)).Handle)
	return reg
}

func TestHandle_WrongRequestType(t *testing.T) {
	f := &fakeCounter{}
	h := newHandler(NewUsecase(f))
	for _, req := range []any{"string", &contentcount.Request{ProjectIDs: []string{idA}}, nil} {
		res, err := h.Handle(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) || res != nil {
			t.Fatalf("req=%T res=%v err=%v", req, res, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestHandle_ReturnsResponseByValue(t *testing.T) {
	f := &fakeCounter{ret: map[string]int{idA: 3}}
	res, err := bus.CallTyped[contentcount.Response](context.Background(), newReg(f),
		constants.BUS_PROJECT_APISPEC_COUNT_SPECS, contentcount.Request{ProjectIDs: []string{idA}})
	if err != nil || res.Counts[idA] != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandle_PropagatesInvalidError(t *testing.T) {
	_, err := bus.CallTyped[contentcount.Response](context.Background(), newReg(&fakeCounter{}),
		constants.BUS_PROJECT_APISPEC_COUNT_SPECS, contentcount.Request{ProjectIDs: manyIDs(101)})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestHandle_ErrorReturnsNilResponse(t *testing.T) {
	h := newHandler(NewUsecase(&fakeCounter{err: errors.New("boom")}))
	res, err := h.Handle(context.Background(), contentcount.Request{ProjectIDs: []string{idA}})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{Bus: reg})
	_, err := reg.Call(context.Background(), constants.BUS_PROJECT_APISPEC_COUNT_SPECS, "bukan-request")
	if errors.Is(err, bus.ErrNotRegistered) || !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}
