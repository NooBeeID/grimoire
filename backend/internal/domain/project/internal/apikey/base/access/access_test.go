package access

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
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

func TestRequire_Allowed(t *testing.T) {
	for _, lv := range []checkaccess.Level{checkaccess.LevelMember, checkaccess.LevelOwner, checkaccess.LevelAdmin} {
		var got checkaccess.Request
		c := New(withHandler(checkaccess.Response{ProjectExists: true, Level: lv}, nil, &got))
		if err := c.Require(context.Background(), "p1", base.Actor{ID: "u1", Role: "developer"}); err != nil {
			t.Fatalf("%s: %v", lv, err)
		}
		if got != (checkaccess.Request{ProjectID: "p1", UserID: "u1", UserRole: "developer"}) {
			t.Fatalf("request=%+v", got)
		}
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
			err := New(withHandler(res, nil, nil)).Require(context.Background(), "p", base.Actor{ID: "u"})
			if !errors.Is(err, base.ErrProjectNotFound) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestRequire_FailClosed(t *testing.T) {
	boom := errors.New("boom")
	err := New(withHandler(checkaccess.Response{}, boom, nil)).Require(context.Background(), "p", base.Actor{ID: "u"})
	var e *errs.Error
	if err == nil || errors.As(err, &e) || !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	err = New(registry()).Require(context.Background(), "p", base.Actor{ID: "u"})
	if !errors.Is(err, bus.ErrNotRegistered) || errors.As(err, &e) {
		t.Fatalf("err=%v", err)
	}
}
