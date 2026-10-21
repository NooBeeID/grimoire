package touchlastused

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apikey/touch"
)

func newBus(r *fakeRepo) *bus.Registry {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bus.NewRegistry(log)
	uc := NewUsecase(r, func() time.Time { return t0 }, log)
	b.Register(constants.BUS_PROJECT_APIKEY_TOUCH, newHandler(uc).Handle)
	return b
}

func TestHandler_PublishValue(t *testing.T) {
	r := &fakeRepo{n: 1}
	b := newBus(r)
	if err := b.Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: validID}); err != nil {
		t.Fatal(err)
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.calls != 1 {
		t.Fatalf("calls=%d", r.calls)
	}
}

func TestHandler_BadRequestType(t *testing.T) {
	for _, req := range []any{&touch.Request{KeyID: validID}, "x", nil, struct{ KeyID string }{validID}} {
		r := &fakeRepo{}
		b := newBus(r)
		if err := b.Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, req); err != nil {
			t.Fatalf("%T: publish is fire-and-forget: %v", req, err)
		}
		_ = b.Shutdown(context.Background())
		h := newHandler(NewUsecase(r, func() time.Time { return t0 }, slog.New(slog.NewTextHandler(io.Discard, nil)))).Handle
		_, err := h(context.Background(), req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
		if r.calls != 0 {
			t.Fatalf("%T: repo called", req)
		}
	}
}

func TestHandler_RepoErrorSwallowed(t *testing.T) {
	r := &fakeRepo{err: errors.New("boom")}
	if err := newBus(r).Publish(context.Background(), constants.BUS_PROJECT_APIKEY_TOUCH, touch.Request{KeyID: validID}); err != nil {
		t.Fatal(err)
	}
}

func TestHandler_DuplicateRegisterPanics(t *testing.T) {
	b := newBus(&fakeRepo{})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	b.Register(constants.BUS_PROJECT_APIKEY_TOUCH, newHandler(NewUsecase(&fakeRepo{}, nil, slog.Default())).Handle)
}
