package resetpassword

import (
	"context"
	"errors"
	"fmt"
	"io"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Repository adalah akses data yang dibutuhkan usecase.
type Repository interface {
	FindUserByID(ctx context.Context, id string) (base.User, bool, error)
	Reset(ctx context.Context, in ResetInput) (base.User, int64, error)
}

// Usecase mengorkestrasi reset password oleh super_admin.
type Usecase struct {
	repo   Repository
	hasher base.Hasher
	audit  audit.Recorder
	clock  base.Clock
	rand   io.Reader
	policy base.PasswordPolicy
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, clock base.Clock, rnd io.Reader, policy base.PasswordPolicy) *Usecase {
	return &Usecase{repo: repo, hasher: hasher, audit: a, clock: clock, rand: rnd, policy: policy}
}

var errSelfReset = errs.WithDetails(errs.CodeForbidden,
	"gunakan ganti password untuk akun sendiri",
	map[string]any{"reason": base.ReasonSelfAction})

// Execute mereset password target dan mencabut semua sesinya.
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error) {
	actor, ok := reqctx.IdentityFrom(ctx)
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	target, found, err := u.repo.FindUserByID(ctx, in.TargetID)
	if err != nil {
		return Result{}, fmt.Errorf("resetpassword: cari user: %w", err)
	}
	if !found {
		return Result{}, base.ErrUserNotFound
	}
	if target.ID == actor.UserID {
		return Result{}, errSelfReset
	}

	pw := in.Password.Reveal()
	generated := pw == ""
	if generated {
		pw, err = base.GeneratePassword(u.rand)
		if err != nil {
			return Result{}, fmt.Errorf("resetpassword: generate password: %w", err)
		}
	} else if fe := u.policy.ValidateField("password", pw, target.Email); len(fe) > 0 {
		return Result{}, base.InvalidFields(fe)
	}

	hash, err := u.hasher.Hash(pw)
	if err != nil {
		return Result{}, fmt.Errorf("resetpassword: hash: %w", err)
	}
	updated, revoked, err := u.repo.Reset(ctx, ResetInput{UserID: target.ID, Hash: hash, Now: u.clock()})
	if err != nil {
		if errors.Is(err, ErrTargetGone) {
			return Result{}, base.ErrUserNotFound
		}
		return Result{}, fmt.Errorf("resetpassword: simpan: %w", err)
	}

	u.audit.Record(ctx, audit.Event{
		Action: "auth.password.reset", TargetType: "user", Target: target.ID,
		Args:   map[string]any{"generated": generated, "revoked_sessions": revoked},
		Status: "ok", Meta: in.Meta,
	})

	res := Result{User: updated.ToResponse()}
	if generated {
		res.TemporaryPassword = &pw
	}
	return res, nil
}
