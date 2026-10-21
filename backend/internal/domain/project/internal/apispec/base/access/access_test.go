package access

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/project/management/checkaccess"
)

func registry(t *testing.T, h bus.Handler) *bus.Registry {
	t.Helper()
	r := bus.NewRegistry(slog.Default())
	if h != nil {
		r.Register(constants.BUS_PROJECT_MANAGEMENT_CHECK_ACCESS, h)
	}
	return r
}

func ctxWithUser() context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: "developer", Email: "a@b"})
}

func respond(res checkaccess.Response, err error) bus.Handler {
	return func(ctx context.Context, req any) (any, error) { return res, err }
}

func TestRequireMember(t *testing.T) {
	cases := []struct {
		name string
		res  checkaccess.Response
		want error
	}{
		{"project tidak ada", checkaccess.Response{ProjectExists: false, Level: checkaccess.LevelMember}, base.ErrProjectNotFound},
		{"level none", checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}, base.ErrProjectNotFound},
		{"level asing", checkaccess.Response{ProjectExists: true, Level: "weird"}, base.ErrProjectNotFound},
		{"member", checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}, nil},
		{"owner", checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelOwner}, nil},
		{"admin", checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelAdmin}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := New(registry(t, respond(c.res, nil))).RequireMember(ctxWithUser(), "p1")
			if !errors.Is(err, c.want) && !(err == nil && c.want == nil) {
				t.Fatalf("got %v want %v", err, c.want)
			}
		})
	}
}

func TestRequireMember_SendsIdentity(t *testing.T) {
	var got checkaccess.Request
	h := func(ctx context.Context, req any) (any, error) {
		got = req.(checkaccess.Request)
		return checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelMember}, nil
	}
	if err := New(registry(t, h)).RequireMember(ctxWithUser(), "p1"); err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != "p1" || got.UserID != "u1" || got.UserRole != "developer" {
		t.Fatalf("%+v", got)
	}
}

func TestRequireMember_FailClosed(t *testing.T) {
	boom := errors.New("boom")
	if err := New(registry(t, respond(checkaccess.Response{}, boom))).RequireMember(ctxWithUser(), "p"); !errors.Is(err, boom) {
		t.Fatalf("handler error harus diteruskan: %v", err)
	}
	err := New(registry(t, nil)).RequireMember(ctxWithUser(), "p")
	if err == nil || !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("tidak terdaftar harus error: %v", err)
	}
	var e *errs.Error
	if errors.As(err, &e) && e.Code == errs.CodeNotFound {
		t.Fatal("kegagalan infra tidak boleh jadi 404")
	}
}

func TestRequireMember_NoIdentity(t *testing.T) {
	err := New(registry(t, respond(checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelAdmin}, nil))).RequireMember(context.Background(), "p")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatalf("%v", err)
	}
}
