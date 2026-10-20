package getme

import (
	"context"
	"errors"
	"testing"

	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

func TestUsecase_Execute_ReturnsIdentity(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{
		UserID: "u1", Email: "a@b.id", Name: "A", Role: "developer",
	})
	me, err := NewUsecase().Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if me.ID != "u1" || me.Email != "a@b.id" || me.Name != "A" || me.Role != "developer" || me.MustChangePassword {
		t.Fatalf("%+v", me)
	}
}

func TestUsecase_Execute_MustChangePasswordTrue(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", MustChangePassword: true})
	me, err := NewUsecase().Execute(ctx)
	if err != nil || !me.MustChangePassword {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestUsecase_Execute_NoIdentity(t *testing.T) {
	_, err := NewUsecase().Execute(context.Background())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatalf("err=%v", err)
	}
}
