package bussearch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listfiles"
	"grimoire/lib/contracts/project/knowledge/search"
)

func newReg(s runner) *bus.Registry {
	reg := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := &Handler{uc: NewUsecase(s)}
	reg.Register(constants.BUS_PROJECT_KNOWLEDGE_SEARCH, h.Handle)
	return reg
}

func TestHandlerSuccess(t *testing.T) {
	reg := newReg(&fakeStore{hits: []base.SearchHit{{Path: "a.md", Headline: "\x01login\x02"}}, total: 1})
	res, err := bus.CallTyped[search.Response](context.Background(), reg, constants.BUS_PROJECT_KNOWLEDGE_SEARCH,
		search.Request{ProjectID: pid(), Query: "login"})
	if err != nil || res.Total != 1 || len(res.Items) != 1 || res.Items[0].Snippet != "**login**" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandlerBadType(t *testing.T) {
	f := &fakeStore{}
	reg := newReg(f)
	for _, req := range []any{"string", listfiles.Request{}, &search.Request{}} {
		_, err := reg.Call(context.Background(), constants.BUS_PROJECT_KNOWLEDGE_SEARCH, req)
		if !errors.Is(err, bus.ErrBadRequestType) {
			t.Fatalf("%T: %v", req, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("store dipanggil")
	}
}
