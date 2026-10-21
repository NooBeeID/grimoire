package verifykey

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/apikey/touch"
	"grimoire/lib/contracts/project/apikey/verify"
)

func newReg(f *fakeRepo) *bus.Registry {
	r := bus.NewRegistry(nil)
	r.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(NewUsecase(f)).Handle)
	return r
}

func TestHandler_WrongTypeRejected(t *testing.T) {
	raw := rawKey(t)
	f := &fakeRepo{}
	r := newReg(f)
	for _, req := range []any{&verify.Request{}, verify.Response{}, raw.Full, nil, touch.Request{}} {
		_, err := r.Call(context.Background(), constants.BUS_PROJECT_APIKEY_VERIFY, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("req %T: err=%v", req, err)
		}
	}
	if f.calls != 0 {
		t.Fatalf("repo calls=%d", f.calls)
	}
}

func TestHandler_ReturnsResponseValue(t *testing.T) {
	raw := rawKey(t)
	r := newReg(&fakeRepo{key: keyFor(raw, true), found: true})
	res, err := bus.CallTyped[verify.Response](context.Background(), r, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw.Full})
	if err != nil || !res.Found || res.KeyID != keyID || res.ProjectID != projectID {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	any1, _ := newHandler(NewUsecase(&fakeRepo{})).Handle(context.Background(), verify.Request{RawKey: "x"})
	if _, ok := any1.(verify.Response); !ok {
		t.Fatalf("type %T", any1)
	}
}

func TestHandler_PropagatesErrorFailClosed(t *testing.T) {
	raw := rawKey(t)
	sentinel := errors.New("db down")
	r := newReg(&fakeRepo{err: sentinel})
	res, err := bus.CallTyped[verify.Response](context.Background(), r, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw.Full})
	if !errors.Is(err, sentinel) || res.Found {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_SingleRegistration(t *testing.T) {
	r := newReg(&fakeRepo{})
	if _, err := r.Call(context.Background(), constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: "x"}); errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal("handler tidak terdaftar")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("registrasi ganda harus panic")
		}
	}()
	r.Register(constants.BUS_PROJECT_APIKEY_VERIFY, newHandler(NewUsecase(&fakeRepo{})).Handle)
}
