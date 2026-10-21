package scanentries

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/contracts/core/audit/scanentries"
)

type fakeScanner struct {
	calls int
	resp  scanentries.Response
	err   error
}

func (f *fakeScanner) Execute(context.Context, scanentries.Request) (scanentries.Response, error) {
	f.calls++
	return f.resp, f.err
}

func TestHandler_WrongType(t *testing.T) {
	s := &fakeScanner{}
	h := handler{uc: s}
	for _, in := range []any{&scanentries.Request{}, "s", nil} {
		if _, err := h.Handle(context.Background(), in); !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("in=%T err=%v", in, err)
		}
	}
	if s.calls != 0 {
		t.Fatal("called")
	}
}

func TestHandler_ReturnsValue(t *testing.T) {
	h := handler{uc: &fakeScanner{resp: scanentries.Response{Entries: []scanentries.Entry{{ID: "x"}}}}}
	out, err := h.Handle(context.Background(), validReq())
	resp, ok := out.(scanentries.Response)
	if err != nil || !ok || len(resp.Entries) != 1 {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}

func TestHandler_Error(t *testing.T) {
	boom := errors.New("boom")
	h := handler{uc: &fakeScanner{err: boom}}
	out, err := h.Handle(context.Background(), validReq())
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if r, ok := out.(scanentries.Response); !ok || r.Entries != nil {
		t.Fatalf("out=%#v", out)
	}
}
