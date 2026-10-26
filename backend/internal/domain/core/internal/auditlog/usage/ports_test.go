package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/knowledge/listfiles"
	"grimoire/lib/contracts/project/management/getproject"
)

func newBus() *bus.Registry { return bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil))) }

func TestProjectFinder_FoundAndMissing(t *testing.T) {
	b := newBus()
	var got getproject.Request
	found := true
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(_ context.Context, req any) (any, error) {
		got = req.(getproject.Request)
		return getproject.Response{Found: found}, nil
	})
	f := NewProjectFinder(b)
	ok, err := f.Exists(context.Background(), "p1")
	if err != nil || !ok || got.ProjectID != "p1" {
		t.Fatalf("ok=%v err=%v got=%+v", ok, err, got)
	}
	found = false
	if ok, err := f.Exists(context.Background(), "p2"); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestProjectFinder_BusError(t *testing.T) {
	b := newBus()
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(context.Context, any) (any, error) {
		return nil, errors.New("down")
	})
	if _, err := NewProjectFinder(b).Exists(context.Background(), "p"); err == nil {
		t.Fatal("ingin error")
	}
}

func TestFileLister_Paginates(t *testing.T) {
	b := newBus()
	var reqs []listfiles.Request
	sizes := []int{500, 500, 10}
	b.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, func(_ context.Context, req any) (any, error) {
		r := req.(listfiles.Request)
		reqs = append(reqs, r)
		n := sizes[len(reqs)-1]
		items := make([]listfiles.Item, n)
		for i := range items {
			items[i] = listfiles.Item{Path: "f", UpdatedAt: time.Date(2026, 1, 1, 7, 0, 0, 0, time.FixedZone("x", 7*3600))}
		}
		return listfiles.Response{Items: items, Truncated: len(reqs) < 3}, nil
	})
	files, capped, err := NewFileLister(b).ListAll(context.Background(), "p")
	if err != nil || capped || len(files) != 1010 {
		t.Fatalf("len=%d capped=%v err=%v", len(files), capped, err)
	}
	for i, r := range reqs {
		if r.Offset != i*500 || !r.Recursive || r.Limit != 500 || r.ProjectID != "p" {
			t.Fatalf("req %d: %+v", i, r)
		}
	}
	if files[0].UpdatedAt.Location() != time.UTC {
		t.Fatal("UpdatedAt bukan UTC")
	}
}

func TestFileLister_StopsAtCap(t *testing.T) {
	b := newBus()
	calls := 0
	b.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_FILES, func(context.Context, any) (any, error) {
		calls++
		return listfiles.Response{Items: make([]listfiles.Item, 500), Truncated: true}, nil
	})
	files, capped, err := NewFileLister(b).ListAll(context.Background(), "p")
	if err != nil || !capped || len(files) != base.MaxScanFiles || calls != 4 {
		t.Fatalf("len=%d capped=%v calls=%d err=%v", len(files), capped, calls, err)
	}
}

func TestFileLister_Error(t *testing.T) {
	if _, _, err := NewFileLister(newBus()).ListAll(context.Background(), "p"); err == nil {
		t.Fatal("ingin error (tidak terdaftar)")
	}
}
