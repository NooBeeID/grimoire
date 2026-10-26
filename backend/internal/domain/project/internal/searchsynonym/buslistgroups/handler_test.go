package buslistgroups

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/searchsynonym/listgroups"
)

func newReg(repo Repository) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := &Handler{uc: New(repo)}
	reg.Register(constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, h.Handle)
	return reg
}

func TestHandlerBadType(t *testing.T) {
	repo := &fakeRepo{}
	reg := newReg(repo)
	for _, req := range []any{struct{}{}, "x", &listgroups.Request{}} {
		_, err := bus.CallTyped[listgroups.Response](context.Background(), reg, constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if repo.called != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestHandlerInvalidProject(t *testing.T) {
	_, err := bus.CallTyped[listgroups.Response](context.Background(), newReg(&fakeRepo{}),
		constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, listgroups.Request{ProjectID: "x"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestHandlerSuccess(t *testing.T) {
	reg := newReg(&fakeRepo{groups: [][]string{{"a", "b"}}})
	res, err := bus.CallTyped[listgroups.Response](context.Background(), reg,
		constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, listgroups.Request{ProjectID: uuid.NewString()})
	if err != nil || len(res.Groups) != 1 || res.Groups[0][1] != "b" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandlerNotRegistered(t *testing.T) {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := bus.CallTyped[listgroups.Response](context.Background(), reg, constants.BUS_PROJECT_SEARCHSYNONYM_LIST_GROUPS, listgroups.Request{})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("got %v", err)
	}
}
