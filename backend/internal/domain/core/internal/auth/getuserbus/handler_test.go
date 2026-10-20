package getuserbus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/getuser"
)

func newBus(repo userFinder) *bus.Registry {
	r := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.Register(constants.BUS_CORE_AUTH_GET_USER, NewHandler(NewUsecase(repo)).Handle)
	return r
}

func TestHandler_Handle_ReturnsValueResponse(t *testing.T) {
	r := newBus(&fakeRepo{user: base.User{ID: uid, Name: "A", IsActive: true}, found: true})
	res, err := bus.CallTyped[getuser.Response](context.Background(), r, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: uid})
	if err != nil || !res.Found || res.ID != uid {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandler_Handle_BadRequestType(t *testing.T) {
	r := newBus(&fakeRepo{})
	for _, req := range []any{"string-salah", &getuser.Request{}} {
		_, err := r.Call(context.Background(), constants.BUS_CORE_AUTH_GET_USER, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
}

func TestHandler_Handle_NotFoundIsNotError(t *testing.T) {
	r := newBus(&fakeRepo{})
	res, err := bus.CallTyped[getuser.Response](context.Background(), r, constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: "x"})
	if err != nil || res.Found {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandler_Handle_PropagatesInfraError(t *testing.T) {
	boom := errors.New("db down")
	r := newBus(&fakeRepo{err: boom})
	_, err := r.Call(context.Background(), constants.BUS_CORE_AUTH_GET_USER, getuser.Request{UserID: uid})
	if !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func TestInit_RegistersEndpoint(t *testing.T) {
	r := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	d := app.Deps{Bus: r}
	Init(d, base.Shared{})
	_, err := r.Call(context.Background(), constants.BUS_CORE_AUTH_GET_USER, "salah")
	if errors.Is(err, bus.ErrNotRegistered) || !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("%v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("want panic on double Init")
		}
	}()
	Init(d, base.Shared{})
}
