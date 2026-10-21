package busreadfile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/readfile"
)

func newReg() (*bus.Registry, *fakeFiles) {
	f := newFake().put(pidA, "docs/a.md", "# A\n")
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg.Register(constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, NewHandler(NewUsecase(f, base.Limits{MaxReadBytes: 1 << 20})).Handle)
	return reg, f
}

func TestHandlerSuccess(t *testing.T) {
	reg, _ := newReg()
	res, err := bus.CallTyped[readfile.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
		readfile.Request{ProjectID: pidA, Path: "/docs/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Found || res.Content != "# A\n" || res.Path != "docs/a.md" {
		t.Fatalf("%+v", res)
	}
}

func TestHandlerBadType(t *testing.T) {
	reg, f := newReg()
	for _, req := range []any{struct{}{}, "x", &readfile.Request{}} {
		_, err := bus.CallTyped[readfile.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if f.finds != 0 {
		t.Fatal("usecase dipanggil")
	}
}

func TestHandlerInvalid(t *testing.T) {
	reg, _ := newReg()
	_, err := bus.CallTyped[readfile.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE,
		readfile.Request{ProjectID: "x", Path: "a.md"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestHandlerNotRegistered(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := bus.CallTyped[readfile.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, readfile.Request{})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("got %v", err)
	}
}
