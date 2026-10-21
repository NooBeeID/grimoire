package listspecsbus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/listendpoints"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

func newReg(f *fakeLister) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg.Register(constants.BUS_PROJECT_APISPEC_LIST_SPECS, newHandler(NewUsecase(f)).Handle)
	return reg
}

func TestT06_AC7_Handle_WrongRequestType(t *testing.T) {
	f := &fakeLister{}
	h := newHandler(NewUsecase(f))
	for _, req := range []any{&listspecs.Request{ProjectID: pid}, "string", nil, listendpoints.Request{}} {
		res, err := h.Handle(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) || res != nil {
			t.Fatalf("req=%T res=%v err=%v", req, res, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("store dipanggil")
	}
}

func TestHandle_ReturnsValueNotPointer(t *testing.T) {
	f := &fakeLister{items: []base.SpecSummary{{Name: "auth"}}}
	res, err := bus.CallTyped[listspecs.Response](context.Background(), newReg(f), constants.BUS_PROJECT_APISPEC_LIST_SPECS, listspecs.Request{ProjectID: pid})
	if err != nil || len(res.Items) != 1 || res.Items[0].Name != "auth" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandle_InvalidProjectID(t *testing.T) {
	_, err := bus.CallTyped[listspecs.Response](context.Background(), newReg(&fakeLister{}), constants.BUS_PROJECT_APISPEC_LIST_SPECS, listspecs.Request{ProjectID: "x"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestHandle_ErrorReturnsNilResponse(t *testing.T) {
	h := newHandler(NewUsecase(&fakeLister{err: errors.New("boom")}))
	res, err := h.Handle(context.Background(), listspecs.Request{ProjectID: pid})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{Bus: reg})
	_, err := reg.Call(context.Background(), constants.BUS_PROJECT_APISPEC_LIST_SPECS, "bukan-request")
	if errors.Is(err, bus.ErrNotRegistered) || !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}
