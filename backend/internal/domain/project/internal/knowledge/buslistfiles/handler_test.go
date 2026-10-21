package buslistfiles

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listfiles"
)

func newReg(repo Repository) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := &Handler{uc: New(repo)}
	reg.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, h.Handle)
	return reg
}

func TestHandlerBadType(t *testing.T) {
	repo := &fakeRepo{}
	reg := newReg(repo)
	for _, req := range []any{struct{}{}, "x", &listfiles.Request{}} {
		_, err := bus.CallTyped[listfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if repo.calls() != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestHandlerSuccess(t *testing.T) {
	reg := newReg(&fakeRepo{metas: metas("docs/a.md", "docs/x/y.md")})
	res, err := bus.CallTyped[listfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
		listfiles.Request{ProjectID: pid(), Prefix: "/docs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Path != "docs/a.md" || len(res.Folders) != 1 || res.Folders[0] != "docs/x" {
		t.Fatalf("%+v", res)
	}
}

func TestHandlerInvalid(t *testing.T) {
	reg := newReg(&fakeRepo{})
	_, err := bus.CallTyped[listfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES,
		listfiles.Request{ProjectID: "x"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestHandlerNotRegistered(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := bus.CallTyped[listfiles.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, listfiles.Request{})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("got %v", err)
	}
}
