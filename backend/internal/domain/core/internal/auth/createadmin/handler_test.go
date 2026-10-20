package createadmin

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/auth/createadmin"
)

func busEnv(t *testing.T) (*bus.Registry, *fakeRepo, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	b := bus.NewRegistry(log)
	r := &fakeRepo{}
	uc := NewUsecase(r, &fakeHasher{}, audit.New(b, log), base.PasswordPolicy{MinLength: 10})
	b.Register(constants.BUS_CORE_AUTH_CREATE_ADMIN, (&handler{uc: uc}).handle)
	return b, r, buf
}

func TestHandler_RegisteredAndTyped(t *testing.T) {
	b, _, _ := busEnv(t)
	res, err := bus.CallTyped[createadmin.Response](context.Background(), b, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	if err != nil || res.UserID != "new-id" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_WrongRequestType(t *testing.T) {
	b, _, _ := busEnv(t)
	_, err := b.Call(context.Background(), constants.BUS_CORE_AUTH_CREATE_ADMIN, "string-salah")
	if !errors.Is(err, bus.ErrBadRequestType) {
		t.Fatalf("err = %v", err)
	}
}

func TestHandler_ConflictVisibleThroughBus(t *testing.T) {
	b, r, _ := busEnv(t)
	r.err = base.ErrEmailTaken
	_, err := b.Call(context.Background(), constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	var ee *errs.Error
	if !errors.As(err, &ee) || ee.Code != errs.CodeConflict {
		t.Fatalf("err = %v", err)
	}
}

func TestHandler_DoesNotLogRequest(t *testing.T) {
	b, r, buf := busEnv(t)
	ctx := context.Background()
	_, _ = b.Call(ctx, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	r.err = errors.New("boom")
	_, _ = b.Call(ctx, constants.BUS_CORE_AUTH_CREATE_ADMIN, validReq())
	if bytes.Contains(buf.Bytes(), []byte("kata-sandi-panjang")) {
		t.Errorf("password bocor ke log: %s", buf)
	}
}

func TestInit_DuplicateRegistrationPanics(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	d := app.Deps{Bus: bus.NewRegistry(log), Log: log}
	s := base.Shared{Hasher: base.Argon2Hasher{Rand: rand.Reader}, Password: base.PasswordPolicy{MinLength: 10}}
	Init(d, s)
	defer func() {
		if recover() == nil {
			t.Error("harus panic")
		}
	}()
	Init(d, s)
}
