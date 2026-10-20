package record

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

func TestHandler_WrongTypeRejected(t *testing.T) {
	r := &fakeRepo{}
	h := handler{uc: newUC(r)}
	for _, in := range []any{&record.Request{}, record.Response{}, "string", nil} {
		_, err := h.Handle(context.Background(), in)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("in=%T err=%v", in, err)
		}
	}
	if r.calls != 0 {
		t.Fatal("repo called")
	}
}

func TestHandler_ReturnsResponseValue(t *testing.T) {
	h := handler{uc: newUC(&fakeRepo{})}
	out, err := h.Handle(context.Background(), validReq())
	if err != nil {
		t.Fatal(err)
	}
	resp, ok := out.(record.Response)
	if !ok || !resp.Recorded {
		t.Fatalf("out=%#v", out)
	}
}

func TestHandler_PropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	h := handler{uc: newUC(&fakeRepo{err: sentinel})}
	out, err := h.Handle(context.Background(), validReq())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v", err)
	}
	if resp, _ := out.(record.Response); resp.Recorded {
		t.Fatal("recorded true on error")
	}
	bad := validReq()
	bad.Action = ""
	if _, err := h.Handle(context.Background(), bad); !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestInit_SingleRegistration(t *testing.T) {
	reg := bus.NewRegistry(nil)
	d := app.Deps{Bus: reg} // DB nil: hanya registrasi, tidak ada Call
	Init(d)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic on double registration")
			}
		}()
		Init(d)
	}()

	// Endpoint terdaftar: Call/Publish tidak mengembalikan ErrNotRegistered
	// (request tipe salah ditolak handler sebelum menyentuh DB).
	_, err := reg.Call(context.Background(), constants.BUS_CORE_AUDIT_RECORD, "x")
	if errors.Is(err, bus.ErrNotRegistered) || !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("call err=%v", err)
	}
	if err := reg.Publish(context.Background(), constants.BUS_CORE_AUDIT_RECORD, "x"); err != nil {
		t.Fatalf("publish err=%v", err)
	}
	_ = reg.Shutdown(context.Background())
}
