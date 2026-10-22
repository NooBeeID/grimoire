package access

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

func reg(res checkaccess.Response, err error) *bus.Registry {
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, func(context.Context, any) (any, error) { return res, err })
	return b
}

func authed() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Name: "Budi"})
}

func TestRequire(t *testing.T) {
	for _, lv := range []checkaccess.Level{checkaccess.LevelMember, checkaccess.LevelOwner, checkaccess.LevelAdmin} {
		a, err := New(reg(checkaccess.Response{ProjectExists: true, Level: lv}, nil)).Require(authed(), "p")
		if err != nil || a != (base.Actor{ID: "u1", Role: "developer", Label: "Budi"}) {
			t.Fatalf("%s: %+v %v", lv, a, err)
		}
	}
	for name, res := range map[string]checkaccess.Response{
		"none": {ProjectExists: true, Level: checkaccess.LevelNone}, "absent": {ProjectExists: false, Level: checkaccess.LevelAdmin}, "weird": {ProjectExists: true, Level: "x"},
	} {
		if _, err := New(reg(res, nil)).Require(authed(), "p"); err != base.ErrProjectNotFound {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := New(reg(checkaccess.Response{}, nil)).Require(context.Background(), "p"); err != base.ErrUnauthenticated {
		t.Error("tanpa login")
	}
	boom := errors.New("boom")
	if _, err := New(reg(checkaccess.Response{}, boom)).Require(authed(), "p"); !errors.Is(err, boom) {
		t.Error("fail-closed")
	}
}
