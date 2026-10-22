package busupsertspecs

import (
	"context"
	"errors"
	"testing"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/upsertspecs"
)

type fakeExec struct {
	calls int
	res   upsertspecs.Response
	err   error
}

func (f *fakeExec) Execute(context.Context, upsertspecs.Request) (upsertspecs.Response, error) {
	f.calls++
	return f.res, f.err
}

func TestHandler_WrongType(t *testing.T) {
	f := &fakeExec{}
	h := newHandler(f)
	if _, err := h.Handle(context.Background(), &upsertspecs.Request{}); err == nil {
		t.Fatal("pointer harus ditolak")
	}
	if _, err := h.Handle(context.Background(), "x"); err == nil {
		t.Fatal("tipe lain harus ditolak")
	}
	if f.calls != 0 {
		t.Fatal("usecase dipanggil")
	}
}

func TestHandler_Delegates(t *testing.T) {
	f := &fakeExec{res: upsertspecs.Response{Summary: upsertspecs.Summary{Created: 2}}}
	out, err := newHandler(f).Handle(context.Background(), upsertspecs.Request{ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	res, ok := out.(upsertspecs.Response)
	if !ok || res.Summary.Created != 2 || f.calls != 1 {
		t.Fatalf("out = %#v", out)
	}
}

func TestHandler_ForwardsError(t *testing.T) {
	want := errs.New(errs.CodeInvalid, "x")
	_, err := newHandler(&fakeExec{err: want}).Handle(context.Background(), upsertspecs.Request{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}
