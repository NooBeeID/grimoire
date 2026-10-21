package users

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
)

func TestGet(t *testing.T) {
	b := bus.NewRegistry(nil)
	b.Register(constants.BUS_CORE_AUTH_GET_USER, func(_ context.Context, req any) (any, error) {
		return getuser.Response{Found: true, ID: req.(getuser.Request).UserID}, nil
	})
	r, err := New(b).Get(context.Background(), "u1")
	if err != nil || !r.Found || r.ID != "u1" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestGet_NotRegistered(t *testing.T) {
	_, err := New(bus.NewRegistry(nil)).Get(context.Background(), "u1")
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal(err)
	}
	_, err = New(bus.NewRegistry(nil)).FindDevelopers(context.Background(), "a", 5)
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatal(err)
	}
}

func TestFindDevelopers(t *testing.T) {
	b := bus.NewRegistry(nil)
	b.Register(constants.BUS_CORE_AUTH_FIND_DEVELOPERS, func(_ context.Context, req any) (any, error) {
		q := req.(finddevelopers.Request)
		return finddevelopers.Response{Items: []finddevelopers.Developer{{ID: q.Query}}}, nil
	})
	items, err := New(b).FindDevelopers(context.Background(), "bud", 50)
	if err != nil || len(items) != 1 || items[0].ID != "bud" {
		t.Fatalf("%+v %v", items, err)
	}
}
