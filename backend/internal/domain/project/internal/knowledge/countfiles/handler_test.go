package countfiles

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/contentcount"
)

func newReg(repo counter) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg.Register(constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, handle(NewUsecase(repo)))
	return reg
}

func TestHandlerBadType(t *testing.T) {
	repo := &fakeRepo{}
	reg := newReg(repo)
	for _, req := range []any{"x", contentcount.Response{}, &contentcount.Request{}} {
		_, err := bus.CallTyped[contentcount.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if len(repo.calls) != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestHandlerSuccess(t *testing.T) {
	a := newID()
	reg := newReg(&fakeRepo{result: map[string]int{a: 4}})
	res, err := bus.CallTyped[contentcount.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
		contentcount.Request{ProjectIDs: []string{a, "bukan-uuid"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts[a] != 4 || res.Counts["bukan-uuid"] != 0 || len(res.Counts) != 2 {
		t.Fatalf("got %v", res.Counts)
	}
}

func TestHandlerRepoError(t *testing.T) {
	sentinel := errors.New("db down")
	reg := newReg(&fakeRepo{err: sentinel})
	_, err := bus.CallTyped[contentcount.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
		contentcount.Request{ProjectIDs: []string{newID()}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v", err)
	}
}

func TestHandlerNotRegistered(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := bus.CallTyped[contentcount.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_COUNT_FILES,
		contentcount.Request{})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("got %v", err)
	}
}
