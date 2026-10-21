package getendpoint

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

func TestHandle_WrongRequestType(t *testing.T) {
	f := one()
	h := newHandler(newUC(f))
	for _, req := range []any{"string", &getendpoint.Request{ProjectID: pidA}, nil} {
		res, err := h.Handle(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) || res != nil {
			t.Fatalf("req=%T res=%v err=%v", req, res, err)
		}
	}
	if f.namesCalls+f.findCalls != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestHandle_ReturnsValueNotPointer(t *testing.T) {
	h := newHandler(newUC(one()))
	res, err := h.Handle(context.Background(), getendpoint.Request{ProjectID: pidA, OperationID: "getPet"})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := res.(getendpoint.Response)
	if !ok || !r.Found {
		t.Fatalf("res=%T %+v", res, res)
	}
}

func TestHandle_ErrorReturnsNil(t *testing.T) {
	res, err := newHandler(newUC(one())).Handle(context.Background(), getendpoint.Request{ProjectID: "x"})
	if err == nil || res != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	Init(app.Deps{Bus: reg})
	_, err := reg.Call(context.Background(), constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, "bukan-request")
	if errors.Is(err, bus.ErrNotRegistered) || !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}
