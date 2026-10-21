// Package revokekey mencabut API key project secara permanen.
package revokekey

import (
	"context"
	"errors"
	"fmt"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/access"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

// txRepo adalah operasi di dalam transaksi. Hanya SQL.
type txRepo interface {
	// LockInProject: SELECT ... WHERE id AND project_id FOR UPDATE. Bukan uuid -> found=false tanpa query.
	LockInProject(ctx context.Context, projectID, keyID string) (base.Key, bool, error)
	// MarkRevoked menyimpan field revoked_* dari k dan mengembalikan baris baru.
	MarkRevoked(ctx context.Context, k base.Key) (base.Key, error)
}

type repository interface {
	InTx(ctx context.Context, fn func(txRepo) error) error
}

// Input adalah parameter usecase.
type Input struct {
	ProjectID string
	KeyID     string
	Meta      base.RequestMeta
}

type Usecase struct {
	repo   repository
	access access.Checker
	audit  audit.Recorder
	clock  base.Clock
}

func NewUsecase(r repository, a access.Checker, au audit.Recorder, c base.Clock) *Usecase {
	return &Usecase{repo: r, access: a, audit: au, clock: c}
}

// Execute mencabut key; audit dikirim setelah commit (fail-open).
func (u *Usecase) Execute(ctx context.Context, in Input) (base.KeyDTO, error) {
	var zero base.KeyDTO
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return zero, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.access.Require(ctx, in.ProjectID, actor); err != nil {
		return zero, err
	}

	var revoked base.Key
	err := u.repo.InTx(ctx, func(tx txRepo) error {
		k, found, err := tx.LockInProject(ctx, in.ProjectID, in.KeyID)
		if err != nil {
			return fmt.Errorf("revokekey: lock key: %w", err)
		}
		if !found {
			return base.ErrKeyNotFound
		}
		if err := k.Revoke(actor, u.clock()); err != nil {
			return err
		}
		revoked, err = tx.MarkRevoked(ctx, k)
		if err != nil {
			var e *errs.Error
			if errors.As(err, &e) {
				return err
			}
			return fmt.Errorf("revokekey: mark revoked: %w", err)
		}
		return nil
	})
	if err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			return zero, err
		}
		return zero, fmt.Errorf("revokekey: tx: %w", err)
	}

	u.audit.Record(ctx, audit.Event{
		Actor: actor, Action: record.ActionAPIKeyRevoked, ProjectID: in.ProjectID, KeyID: revoked.ID,
		Args: map[string]any{"name": revoked.Name, "prefix": revoked.Prefix}, Meta: in.Meta,
	})
	return revoked.ToDTO(), nil
}
