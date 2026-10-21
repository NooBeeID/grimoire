package purgeinvitations

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/countmemberships"
)

func newRegistry() *bus.Registry {
	return bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestBusCounter_NotRegistered(t *testing.T) {
	_, err := busCounter{b: newRegistry()}.Count(context.Background(), []string{"a"})
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("err = %v", err)
	}
}

func TestBusCounter_PassesRequestAndCounts(t *testing.T) {
	reg := newRegistry()
	var got countmemberships.Request
	reg.Register(constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS, func(_ context.Context, req any) (any, error) {
		r, err := bus.Expect[countmemberships.Request](req)
		if err != nil {
			return nil, err
		}
		got = r
		return countmemberships.Response{Counts: map[string]int{"a": 2}}, nil
	})
	counts, err := busCounter{b: reg}.Count(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.UserIDs, []string{"a", "b"}) {
		t.Fatalf("request = %v", got)
	}
	if counts["a"] != 2 || counts["b"] != 0 {
		t.Fatalf("counts = %v", counts)
	}
}

func TestBusCounter_HandlerErrorWrapped(t *testing.T) {
	reg := newRegistry()
	hErr := errs.New(errs.CodeInvalid, "bad")
	reg.Register(constants.BUS_PROJECT_MANAGEMENT_COUNT_MEMBERSHIPS, func(context.Context, any) (any, error) {
		return nil, hErr
	})
	_, err := busCounter{b: reg}.Count(context.Background(), []string{"a"})
	if !errors.Is(err, hErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestBusCounter_UsecaseFailClosedWithRealRegistry(t *testing.T) {
	r := &fakeRepo{batches: [][]Candidate{cand("a")}}
	uc, _ := newUC(r, &fakeCounter{}, fixed)
	uc.counter = busCounter{b: newRegistry()}
	err := uc.Run(context.Background())
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("err = %v", err)
	}
	if len(r.delCalls) != 0 {
		t.Fatal("tidak boleh menghapus")
	}
}
