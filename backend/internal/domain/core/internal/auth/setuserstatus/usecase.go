package setuserstatus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Tx adalah operasi data di dalam satu transaksi.
type Tx interface {
	LockActiveSuperAdminIDs(ctx context.Context) ([]string, error)
	LockUser(ctx context.Context, id string) (base.User, bool, error)
	SetActive(ctx context.Context, id string, active bool, now time.Time) error
	DeleteSessions(ctx context.Context, userID string) (int64, error)
}

// Repository membuka transaksi.
type Repository interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Input adalah masukan Execute. Active=true berarti activate.
type Input struct {
	TargetID string
	Active   bool
	Meta     base.RequestMeta
}

// Usecase mengorkestrasi aktivasi/penonaktifan user.
type Usecase struct {
	repo  Repository
	audit audit.Recorder
	clock base.Clock
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, a audit.Recorder, clock base.Clock) *Usecase {
	return &Usecase{repo: repo, audit: a, clock: clock}
}

// Execute mengubah status aktif target. Urutan kunci deactivate: set super admin dulu
// (urut id), baru baris target, agar dua admin yang saling menonaktifkan tidak deadlock.
func (u *Usecase) Execute(ctx context.Context, in Input) (base.UserDTO, error) {
	actor, ok := reqctx.IdentityFrom(ctx)
	if !ok {
		return base.UserDTO{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	now := u.clock()
	var (
		user    base.User
		changed bool
		revoked int64
	)
	err := u.repo.InTx(ctx, func(tx Tx) error {
		var adminIDs []string
		if !in.Active {
			ids, err := tx.LockActiveSuperAdminIDs(ctx)
			if err != nil {
				return fmt.Errorf("setuserstatus: kunci super admin: %w", err)
			}
			adminIDs = ids
		}
		found, err := lockTarget(ctx, tx, in.TargetID, &user)
		if err != nil {
			return err
		}
		if !found {
			return base.ErrUserNotFound
		}
		if !in.Active {
			if err := base.CheckDeactivate(actor.UserID, user, len(adminIDs)); err != nil {
				return err
			}
		}
		changed = user.ApplyStatus(in.Active, now)
		if !changed {
			return nil
		}
		if err := tx.SetActive(ctx, user.ID, in.Active, now); err != nil {
			return fmt.Errorf("setuserstatus: set status: %w", err)
		}
		if !in.Active {
			revoked, err = tx.DeleteSessions(ctx, user.ID)
			if err != nil {
				return fmt.Errorf("setuserstatus: cabut sesi: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		var appErr *errs.Error
		if errors.As(err, &appErr) {
			return base.UserDTO{}, err
		}
		return base.UserDTO{}, fmt.Errorf("setuserstatus: transaksi: %w", err)
	}

	if changed {
		ev := audit.Event{
			Action: "user.activated", TargetType: "user", Target: user.ID,
			Args: map[string]any{}, Status: "ok", Meta: in.Meta,
		}
		if !in.Active {
			ev.Action = "user.deactivated"
			ev.Args = map[string]any{"revoked_sessions": revoked}
		}
		u.audit.Record(ctx, ev)
	}
	return user.ToResponse(), nil
}

func lockTarget(ctx context.Context, tx Tx, id string, out *base.User) (bool, error) {
	usr, found, err := tx.LockUser(ctx, id)
	if err != nil {
		return false, fmt.Errorf("setuserstatus: kunci user: %w", err)
	}
	*out = usr
	return found, nil
}
