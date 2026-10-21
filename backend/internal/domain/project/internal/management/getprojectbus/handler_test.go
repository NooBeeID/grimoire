package getprojectbus

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/getproject"
)

func newBus(f *fakeFinder) *bus.Registry {
	r := bus.NewRegistry(nil)
	r.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, newHandler(NewUsecase(f)).Handle)
	return r
}

func TestHandler_Handle_ReturnsValueResponse(t *testing.T) {
	r := newBus(&fakeFinder{p: base.Project{ID: "a", Name: "B"}, found: true})
	res, err := bus.CallTyped[getproject.Response](context.Background(), r, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: "a"})
	if err != nil || res != (getproject.Response{Found: true, ID: "a", Name: "B"}) {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestHandler_Handle_BadRequestType(t *testing.T) {
	r := newBus(&fakeFinder{})
	for _, req := range []any{"string-salah", &getproject.Request{}} {
		_, err := r.Call(context.Background(), constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("req %T: err=%v", req, err)
		}
	}
}

func TestHandler_Handle_NotFoundIsNotError(t *testing.T) {
	r := newBus(&fakeFinder{})
	res, err := bus.CallTyped[getproject.Response](context.Background(), r, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: "x"})
	if err != nil || res.Found {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestHandler_Handle_PropagatesInfraError(t *testing.T) {
	boom := errors.New("db down")
	r := newBus(&fakeFinder{err: boom})
	res, err := bus.CallTyped[getproject.Response](context.Background(), r, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: "x"})
	if !errors.Is(err, boom) || res.Found {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	r := bus.NewRegistry(nil)
	d := app.Deps{Bus: r} // DB nil: store.New hanya menyimpan querier, tidak dipakai saat Init
	Init(d)
	// id bukan uuid dijawab store tanpa menyentuh DB.
	res, err := bus.CallTyped[getproject.Response](context.Background(), r, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, getproject.Request{ProjectID: "bukan-uuid"})
	if errors.Is(err, bus.ErrNotRegistered) || err != nil || res.Found {
		t.Fatalf("got %+v %v", res, err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("registrasi ganda harus panik")
		}
	}()
	Init(d)
}
