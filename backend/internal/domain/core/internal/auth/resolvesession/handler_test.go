package resolvesession

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

func newBus(r *fakeRepo) *bus.Registry {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	reg := bus.NewRegistry(log)
	uc := NewUsecase(r, func() time.Time { return testNow }, base.SessionConfig{Secret: testSecret, IdleTimeout: idle}, log)
	reg.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, Handle(uc))
	return reg
}

func TestResolveSession_BadRequestType(t *testing.T) {
	r := validRepo()
	reg := newBus(r)
	for _, req := range []any{"string-salah", &resolvesession.Request{Token: testToken}} {
		_, err := reg.Call(context.Background(), constants.BUS_CORE_AUTH_RESOLVE_SESSION, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("req %T: err %v", req, err)
		}
	}
	if r.finds != 0 {
		t.Fatal("repo tidak boleh dipanggil")
	}
}

func TestResolveSession_CallTypedRoundTrip(t *testing.T) {
	reg := newBus(validRepo())
	got, err := bus.CallTyped[resolvesession.Response](context.Background(), reg,
		constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{Token: testToken})
	if err != nil || !got.Found || got.UserID != "u1" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestResolveSession_EndpointRegistered(t *testing.T) {
	reg := newBus(validRepo())
	_, err := reg.Call(context.Background(), constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
}
