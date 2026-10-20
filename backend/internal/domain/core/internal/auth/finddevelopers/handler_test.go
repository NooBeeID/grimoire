package finddevelopers

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	contract "grimoire/lib/contracts/core/auth/finddevelopers"
)

func newReg(repo developerFinder) *bus.Registry {
	reg := bus.NewRegistry(nil)
	reg.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, NewHandler(NewUsecase(repo)).Handle)
	return reg
}

func TestHandler_Handle_ReturnsValueResponse(t *testing.T) {
	reg := newReg(&fakeRepo{items: []contract.Developer{{ID: "1", Name: "A", Email: "a@x"}}})
	res, err := bus.CallTyped[contract.Response](context.Background(), reg, constants.BUS_CORE_AUTH_FIND_DEVELOPERS, contract.Request{Query: "a"})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandler_Handle_BadRequestType(t *testing.T) {
	reg := newReg(&fakeRepo{})
	for _, req := range []any{"string-salah", &contract.Request{}} {
		_, err := reg.Call(context.Background(), constants.BUS_CORE_AUTH_FIND_DEVELOPERS, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Errorf("%T: %v", req, err)
		}
	}
}

func TestHandler_Handle_EmptyResultIsNotError(t *testing.T) {
	res, err := bus.CallTyped[contract.Response](context.Background(), newReg(&fakeRepo{}), constants.BUS_CORE_AUTH_FIND_DEVELOPERS, contract.Request{Query: "zzz"})
	if err != nil || res.Items == nil || len(res.Items) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandler_Handle_PropagatesInfraError(t *testing.T) {
	boom := errors.New("db down")
	_, err := newReg(&fakeRepo{err: boom}).Call(context.Background(), constants.BUS_CORE_AUTH_FIND_DEVELOPERS, contract.Request{Query: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func TestHandler_Register_DuplicatePanics(t *testing.T) {
	reg := newReg(&fakeRepo{})
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	reg.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, NewHandler(NewUsecase(&fakeRepo{})).Handle)
}
