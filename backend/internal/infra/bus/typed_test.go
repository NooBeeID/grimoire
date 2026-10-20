package bus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

func TestT00_AC3_CallTyped_NilRegistry(t *testing.T) {
	v, err := CallTyped[string](context.Background(), nil, "x.y.z", nil)
	if !errors.Is(err, ErrNilRegistry) || v != "" {
		t.Fatalf("v=%q err=%v", v, err)
	}
}

func TestT00_AC3_CallTyped_WrongResponseType(t *testing.T) {
	r := NewRegistry(nil)
	r.Register("x.y.z", func(context.Context, any) (any, error) { return 1, nil })
	_, err := CallTyped[string](context.Background(), r, "x.y.z", nil)
	if !errors.Is(err, ErrBadResponseType) || !strings.Contains(err.Error(), "want=string got=int") {
		t.Fatalf("err=%v", err)
	}
}

func TestCallTyped_NilResponse(t *testing.T) {
	r := NewRegistry(nil)
	r.Register("x.y.z", noop)
	if _, err := CallTyped[string](context.Background(), r, "x.y.z", nil); !errors.Is(err, ErrBadResponseType) {
		t.Fatalf("err=%v", err)
	}
}

func TestCallTyped_HandlerErrorPassesThrough(t *testing.T) {
	s := errors.New("s")
	r := NewRegistry(nil)
	r.Register("x.y.z", func(context.Context, any) (any, error) { return "ignored", s })
	v, err := CallTyped[string](context.Background(), r, "x.y.z", nil)
	if !errors.Is(err, s) || v != "" {
		t.Fatalf("v=%q err=%v", v, err)
	}
}

func TestCallTyped_Unregistered(t *testing.T) {
	if _, err := CallTyped[string](context.Background(), NewRegistry(nil), "x.y.z", nil); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("err=%v", err)
	}
}

func TestCallTyped_ResolveSessionContract(t *testing.T) {
	want := resolvesession.Response{Found: true, UserID: "u1", Role: resolvesession.RoleDeveloper}
	r := NewRegistry(nil)
	r.Register(constants.BUS_CORE_AUTH_RESOLVE_SESSION, func(_ context.Context, req any) (any, error) {
		in, err := Expect[resolvesession.Request](req)
		if err != nil {
			return nil, err
		}
		if in.Token != "t" {
			return resolvesession.Response{}, nil
		}
		return want, nil
	})
	got, err := CallTyped[resolvesession.Response](context.Background(), r, constants.BUS_CORE_AUTH_RESOLVE_SESSION, resolvesession.Request{Token: "t"})
	if err != nil || got != want {
		t.Fatalf("got=%v err=%v", got, err)
	}
	// request bertipe salah ditolak provider
	_, err = CallTyped[resolvesession.Response](context.Background(), r, constants.BUS_CORE_AUTH_RESOLVE_SESSION, "t")
	if !errors.Is(err, ErrBadRequestType) {
		t.Fatalf("err=%v", err)
	}
}

func TestExpect_WrongRequestType(t *testing.T) {
	_, err := Expect[resolvesession.Request]("x")
	if !errors.Is(err, ErrBadRequestType) || !strings.Contains(err.Error(), "want=resolvesession.Request got=string") {
		t.Fatalf("err=%v", err)
	}
}

func TestExpect_OK(t *testing.T) {
	v, err := Expect[resolvesession.Request](resolvesession.Request{Token: "a"})
	if err != nil || v.Token != "a" {
		t.Fatalf("v=%v err=%v", v, err)
	}
}

func TestExpect_InterfaceType(t *testing.T) {
	_, err := Expect[fmt.Stringer]("x")
	if !errors.Is(err, ErrBadRequestType) || !strings.Contains(err.Error(), "fmt.Stringer") {
		t.Fatalf("err=%v", err)
	}
}
