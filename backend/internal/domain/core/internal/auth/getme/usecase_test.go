package getme

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/infra/mailer/mailertest"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

func TestUsecase_Execute_ReturnsIdentity(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{
		UserID: "u1", Email: "a@b.id", Name: "A", Role: "developer",
	})
	me, err := NewUsecase(&mailertest.Fake{}).Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if me.ID != "u1" || me.Email != "a@b.id" || me.Name != "A" || me.Role != "developer" || me.MustChangePassword {
		t.Fatalf("%+v", me)
	}
}

func TestUsecase_Execute_MustChangePasswordTrue(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", MustChangePassword: true})
	me, err := NewUsecase(&mailertest.Fake{}).Execute(ctx)
	if err != nil || !me.MustChangePassword {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestUsecase_Execute_NoIdentity(t *testing.T) {
	_, err := NewUsecase(&mailertest.Fake{}).Execute(context.Background())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized {
		t.Fatalf("err=%v", err)
	}
}

func TestUsecase_Execute_NilMailerDisabled(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1"})
	me, err := NewUsecase(nil).Execute(ctx)
	if err != nil || me.EmailEnabled || me.ID != "u1" {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestUsecase_Execute_EmailEnabledFollowsMailer(t *testing.T) {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1"})
	on, _ := NewUsecase(&mailertest.Fake{}).Execute(ctx)
	off, _ := NewUsecase(&mailertest.Fake{NotConfigured: true}).Execute(ctx)
	if !on.EmailEnabled || off.EmailEnabled {
		t.Fatalf("on=%v off=%v", on.EmailEnabled, off.EmailEnabled)
	}
}
