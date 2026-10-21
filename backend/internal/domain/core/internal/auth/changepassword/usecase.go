package changepassword

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

const msgInvitationExpired = "password sementara sudah kedaluwarsa, minta pengundang mengirim ulang undangan"

// Repository adalah akses data yang dibutuhkan usecase.
type Repository interface {
	FindUserByID(ctx context.Context, id string) (base.User, bool, error)
	FindInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
	Change(ctx context.Context, in ChangeInput) (revoked int64, accepted bool, err error)
}

// Input adalah masukan Execute.
type Input struct {
	CurrentPassword, NewPassword base.Secret
	CurrentTokenHash             []byte
	Meta                         base.RequestMeta
}

// Usecase mengorkestrasi pergantian password oleh user sendiri.
type Usecase struct {
	repo   Repository
	hasher base.Hasher
	audit  audit.Recorder
	policy base.PasswordPolicy
	clock  base.Clock
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, policy base.PasswordPolicy, clock base.Clock) *Usecase {
	return &Usecase{repo: repo, hasher: hasher, audit: a, policy: policy, clock: clock}
}

func unauthorized() error { return errs.New(errs.CodeUnauthorized, "perlu login") }

// Execute mengganti password user pada ctx dan mencabut sesi lainnya.
func (u *Usecase) Execute(ctx context.Context, in Input) error {
	id, ok := reqctx.IdentityFrom(ctx)
	if !ok || len(in.CurrentTokenHash) == 0 {
		return unauthorized()
	}
	user, found, err := u.repo.FindUserByID(ctx, id.UserID)
	if err != nil {
		return fmt.Errorf("changepassword: cari user: %w", err)
	}
	if !found || !user.CanLogin() {
		return unauthorized()
	}

	now := u.clock()
	inv, hasInv, err := u.repo.FindInvitation(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("changepassword: cari undangan: %w", err)
	}
	if hasInv && inv.IsExpired(now) {
		return base.InvalidFields(base.FieldErrors{"current_password": msgInvitationExpired})
	}

	fe := u.policy.ValidateChange(in.CurrentPassword, in.NewPassword, user.Email)
	if _, bad := fe["current_password"]; !bad {
		okOld, err := u.hasher.Verify(user.PasswordHash, in.CurrentPassword.Reveal())
		if err != nil {
			return fmt.Errorf("changepassword: verifikasi: %w", err)
		}
		if !okOld {
			fe = fe.Merge(base.FieldErrors{"current_password": "password lama salah"})
		} else if e := base.DifferentPassword(in.CurrentPassword, in.NewPassword); e != nil {
			fe = fe.Merge(base.AsFieldErrors(e))
		}
	}
	if len(fe) > 0 {
		return base.InvalidFields(fe)
	}

	newHash, err := u.hasher.Hash(in.NewPassword.Reveal())
	if err != nil {
		return fmt.Errorf("changepassword: hash: %w", err)
	}
	revoked, accepted, err := u.repo.Change(ctx, ChangeInput{
		UserID: user.ID, OldHash: user.PasswordHash, NewHash: newHash,
		KeepTokenHash: in.CurrentTokenHash, Now: now,
	})
	if err != nil {
		if errors.Is(err, ErrStale) {
			return errs.New(errs.CodeConflict, "password berubah oleh proses lain, muat ulang dan coba lagi")
		}
		return fmt.Errorf("changepassword: simpan: %w", err)
	}

	u.audit.Record(ctx, audit.Event{
		Action: "auth.password.changed", TargetType: "user", Target: user.ID,
		Args:   map[string]any{"revoked_sessions": revoked},
		Status: "ok", Meta: in.Meta,
	})
	if accepted {
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionUserInvitationAccepted, TargetType: "user", Target: user.ID,
			Args:   map[string]any{"email": user.Email},
			Status: record.StatusOK, Meta: in.Meta,
		})
	}
	return nil
}
