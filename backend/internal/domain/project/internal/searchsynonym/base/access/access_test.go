package access

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

func registry() *bus.Registry { return bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil))) }

func withHandler(res checkaccess.Response, err error, got *checkaccess.Request) *bus.Registry {
	b := registry()
	b.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(_ context.Context, req any) (any, error) {
		if got != nil {
			*got = req.(checkaccess.Request)
		}
		return res, err
	})
	return b
}

func authed() context.Context {
	return reqctx.WithIdentity(context.Background(),
		reqctx.Identity{UserID: "u1", Role: "developer", Name: "Budi", Email: "b@x"})
}

func TestRequire_Allowed(t *testing.T) {
	for _, lv := range []checkaccess.Level{checkaccess.LevelMember, checkaccess.LevelOwner, checkaccess.LevelAdmin} {
		var got checkaccess.Request
		c := New(withHandler(checkaccess.Response{ProjectExists: true, Level: lv}, nil, &got))
		a, err := c.Require(authed(), "p1")
		if err != nil {
			t.Fatalf("%s: %v", lv, err)
		}
		if a != (base.Actor{ID: "u1", Role: "developer", Label: "Budi"}) {
			t.Fatalf("actor=%+v", a)
		}
		if got != (checkaccess.Request{ProjectID: "p1", UserID: "u1", UserRole: "developer"}) {
			t.Fatalf("request=%+v", got)
		}
	}
}

func TestRequire_LabelFallsBackToEmail(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "b@x"})
	a, err := New(withHandler(checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}, nil, nil)).Require(ctx, "p")
	if err != nil || a.Label != "b@x" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestRequire_Unauthenticated(t *testing.T) {
	_, err := New(registry()).Require(context.Background(), "p")
	if !errors.Is(err, base.ErrUnauthenticated) {
		t.Fatalf("err=%v", err)
	}
}

func TestRequire_NotFound(t *testing.T) {
	cases := map[string]checkaccess.Response{
		"not-exists":    {ProjectExists: false, Level: checkaccess.LevelAdmin},
		"none":          {ProjectExists: true, Level: checkaccess.LevelNone},
		"unknown-level": {ProjectExists: true, Level: "weird"},
		"empty-level":   {ProjectExists: true},
	}
	for name, res := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(withHandler(res, nil, nil)).Require(authed(), "p")
			if !errors.Is(err, base.ErrProjectNotFound) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestRequire_FailClosed(t *testing.T) {
	boom := errors.New("boom")
	_, err := New(withHandler(checkaccess.Response{}, boom, nil)).Require(authed(), "p")
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	_, err = New(registry()).Require(authed(), "p")
	if !errors.Is(err, bus.ErrNotRegistered) || errors.As(err, &e) {
		t.Fatalf("err=%v", err)
	}
}
