package createuser

import (
	"context"
	"errors"
	"fmt"
	"io"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah kebutuhan usecase terhadap penyimpanan.
type Repository interface {
	Insert(ctx context.Context, u base.User) (base.User, error)
}

// Usecase membuat user baru (orkestrasi saja; rule ada di base).
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

// Execute membuat user. Validasi dikumpulkan sebelum hashing/DB.
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error) {
	actor, ok := reqctx.IdentityFrom(ctx)
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	user, fe := base.NewUser(base.NewUserInput{
		Email: in.Email, Name: in.Name, Role: in.Role,
		CreatedBy: actor.UserID, CreatedByLabel: actor.Email, Now: u.clock(),
	})
	pw := in.Password.Reveal()
	generated := pw == ""
	if !generated {
		// Email tidak valid -> pembanding kosong: hanya cek panjang/byte.
		email, _ := base.NormalizeEmail(in.Email)
		fe = fe.Merge(u.policy.ValidateField("password", pw, email))
	}
	if len(fe) > 0 {
		return Result{}, base.InvalidFields(fe)
	}
	if generated {
		var err error
		if pw, err = base.GeneratePassword(u.rand); err != nil {
			return Result{}, fmt.Errorf("createuser: generate password: %w", err)
		}
	}
	hash, err := u.hasher.Hash(pw)
	if err != nil {
		return Result{}, fmt.Errorf("createuser: hash: %w", err)
	}
	user.PasswordHash = hash
	created, err := u.repo.Insert(ctx, user)
	if err != nil {
		if errors.Is(err, base.ErrEmailTaken) {
			return Result{}, base.EmailConflict()
		}
		return Result{}, fmt.Errorf("createuser: insert: %w", err)
	}
	u.audit.Record(ctx, audit.Event{
		Action: "user.created", TargetType: "user", Target: created.ID,
		Args: map[string]any{
			"email": created.Email, "name": created.Name, "role": created.Role,
			"generated_password": generated,
		},
		Status: record.StatusOK, Meta: in.Meta,
	})
	res := Result{User: created.ToResponse()}
	if generated {
		res.TemporaryPassword = &pw
	}
	return res, nil
}
