package createadmin

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/createadmin"
)

const cliLabel = "cli"

// Usecase membuat Super Admin dari CLI (orkestrasi saja; rule ada di base).
type Usecase struct {
	repo   Repository
	hasher base.Hasher
	audit  audit.Recorder
	policy base.PasswordPolicy
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, policy base.PasswordPolicy) *Usecase {
	return &Usecase{repo: repo, hasher: hasher, audit: a, policy: policy}
}

// Create memvalidasi, meng-hash, menyimpan, lalu mencatat audit.
func (u *Usecase) Create(ctx context.Context, in createadmin.Request) (createadmin.Response, error) {
	var fe base.FieldErrors
	email, err := base.NormalizeEmail(in.Email)
	fe = fe.Merge(base.AsFieldErrors(err))
	name, err := base.ValidateName(in.Name)
	fe = fe.Merge(base.AsFieldErrors(err))
	pw := base.Secret(in.Password)
	fe = fe.Merge(u.policy.ValidateField("password", pw.Reveal(), email))
	if len(fe) > 0 {
		return createadmin.Response{}, base.InvalidFields(fe)
	}

	hash, err := u.hasher.Hash(pw.Reveal())
	if err != nil {
		return createadmin.Response{}, fmt.Errorf("createadmin: hash password: %w", err)
	}
	label := cliLabel
	created, err := u.repo.InsertUser(ctx, base.User{
		Email: email, Name: name, PasswordHash: hash,
		Role: base.RoleSuperAdmin, IsActive: true, MustChangePassword: false,
		CreatedByLabel: &label,
	})
	if err != nil {
		if errors.Is(err, base.ErrEmailTaken) {
			return createadmin.Response{}, base.EmailConflict()
		}
		return createadmin.Response{}, fmt.Errorf("createadmin: insert user: %w", err)
	}
	u.audit.Record(ctx, audit.Event{
		Action: "user.created", TargetType: "user", Target: created.ID,
		Args: map[string]any{
			"email": created.Email, "name": created.Name, "role": created.Role,
			"generated_password": false,
		},
		Status: record.StatusOK,
		Actor:  &audit.Actor{Type: record.ActorSystem, Label: cliLabel},
	})
	return createadmin.Response{UserID: created.ID}, nil
}
