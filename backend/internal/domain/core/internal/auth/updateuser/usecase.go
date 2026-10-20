package updateuser

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Repository dipenuhi repository.go. Seluruh SQL ada di sana.
type Repository interface {
	InTx(ctx context.Context, fn func(ctx context.Context, tx TxRepository) error) error
}

// TxRepository adalah akses data di dalam satu transaksi.
type TxRepository interface {
	// LockActiveSuperAdminIDs mengunci super admin aktif, urut id.
	LockActiveSuperAdminIDs(ctx context.Context) ([]string, error)
	// LockUserByID: SELECT ... FOR UPDATE; (zero, false, nil) bila tidak ada.
	LockUserByID(ctx context.Context, id string) (base.User, bool, error)
	// UpdateUserFields menulis name, email, role, updated_at. Email bentrok -> base.ErrEmailTaken.
	UpdateUserFields(ctx context.Context, u base.User) error
}

// Usecase mengorkestrasi perubahan name/email/role oleh super_admin.
type Usecase struct {
	repo  Repository
	audit audit.Recorder
	clock base.Clock
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, rec audit.Recorder, clock base.Clock) *Usecase {
	return &Usecase{repo: repo, audit: rec, clock: clock}
}

// Result adalah hasil UpdateUser. Changed kosong berarti no-op.
type Result struct {
	User    base.User
	Changed []string
}

// UpdateUser menerapkan patch. Urutan lock: set super admin aktif (urut id) lalu target,
// hanya bila patch memuat role.
func (u *Usecase) UpdateUser(ctx context.Context, p Params) (Result, error) {
	actor, ok := reqctx.IdentityFrom(ctx)
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if _, err := uuid.Parse(p.ID); err != nil {
		return Result{}, base.ErrUserNotFound
	}

	var (
		target  base.User
		before  base.User
		changed []string
	)
	err := u.repo.InTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var ids []string
		if p.Role != nil {
			var err error
			if ids, err = tx.LockActiveSuperAdminIDs(ctx); err != nil {
				return err
			}
		}
		t, found, err := tx.LockUserByID(ctx, p.ID)
		if err != nil {
			return err
		}
		if !found {
			return base.ErrUserNotFound
		}
		before = t
		ch, err := t.ApplyUpdate(base.UserPatch{Name: p.Name, Email: p.Email, Role: p.Role})
		if err != nil {
			if fe := base.AsFieldErrors(err); fe != nil {
				return base.InvalidFields(fe)
			}
			return err
		}
		if slices.Contains(ch, "role") {
			if err := base.CheckRoleChange(actor.UserID, before, t.Role, len(ids)); err != nil {
				return err
			}
		}
		target, changed = t, ch
		if len(ch) == 0 {
			return nil
		}
		target.UpdatedAt = u.clock().UTC()
		if err := tx.UpdateUserFields(ctx, target); err != nil {
			if errors.Is(err, base.ErrEmailTaken) {
				return base.EmailConflict()
			}
			return fmt.Errorf("updateuser: update: %w", err)
		}
		return nil
	})
	if err != nil {
		var ee *errs.Error
		if errors.As(err, &ee) {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("updateuser: tx: %w", err)
	}
	if len(changed) == 0 {
		return Result{User: target}, nil
	}

	args := map[string]any{"changed_fields": changed}
	if slices.Contains(changed, "role") {
		args["role_from"], args["role_to"] = before.Role, target.Role
	}
	u.audit.Record(ctx, audit.Event{
		Action: "user.updated", TargetType: "user", Target: target.ID,
		Status: "ok", Meta: p.Meta, Args: args,
	})
	return Result{User: target, Changed: changed}, nil
}
