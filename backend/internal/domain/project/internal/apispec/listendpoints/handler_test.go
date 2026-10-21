package listendpoints

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
	"grimoire/lib/contracts/project/apispec/listendpoints"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

func newReg(f *fakeReader) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg.Register(constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, newHandler(NewUsecase(f)).Handle)
	return reg
}

func TestHandle_WrongRequestType(t *testing.T) {
	f := &fakeReader{}
	h := newHandler(NewUsecase(f))
	for _, req := range []any{"string", &listendpoints.Request{ProjectID: pid}, listspecs.Request{ProjectID: pid}, nil} {
		res, err := h.Handle(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) || res != nil {
			t.Fatalf("req=%T res=%v err=%v", req, res, err)
		}
	}
	if f.qCalls+f.nameCalls != 0 {
		t.Fatal("usecase dipanggil")
	}
}

func TestHandle_ReturnsResponseByValue(t *testing.T) {
	res, err := bus.CallTyped[listendpoints.Response](context.Background(), newReg(&fakeReader{rows: rows(1), total: 1}),
		constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, listendpoints.Request{ProjectID: pid})
	if err != nil || len(res.Items) != 1 || !res.SpecFound {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandle_PropagatesUsecaseError(t *testing.T) {
	_, err := bus.CallTyped[listendpoints.Response](context.Background(), newReg(&fakeReader{}),
		constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, listendpoints.Request{ProjectID: "x"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
	h := newHandler(NewUsecase(&fakeReader{queryErr: errors.New("boom")}))
	res, err := h.Handle(context.Background(), listendpoints.Request{ProjectID: pid})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{Bus: reg})
	_, err := reg.Call(context.Background(), constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, "bukan-request")
	if errors.Is(err, bus.ErrNotRegistered) || !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}
