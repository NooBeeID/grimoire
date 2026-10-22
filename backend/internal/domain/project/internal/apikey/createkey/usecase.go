// Package createkey membuat API key project dan menampilkan plaintext sekali.
package createkey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/domain/project/internal/apikey/base/access"
	"grimoire/internal/domain/project/internal/apikey/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

const maxPrefixAttempts = 3

// Tx = operasi di dalam satu transaksi create. Implementasi: repository.
// ownerID = pemanggil (actor.ID) untuk semua role; kuota dan unik nama per user per project (T13 K5).
type Tx interface {
	LockOwnerKeys(ctx context.Context, projectID, ownerID string) error
	CountActive(ctx context.Context, projectID, ownerID string) (int, error)
	Insert(ctx context.Context, k base.Key) (base.Key, error)
}

type Repository interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
	repo      Repository
	access    access.Checker
	audit     audit.Recorder
	clock     base.Clock
	rnd       io.Reader
	limit     int
	publicURL string
	log       *slog.Logger
}

func NewUsecase(r Repository, a access.Checker, au audit.Recorder, c base.Clock, rnd io.Reader, limit int, publicURL string, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: r, access: a, audit: au, clock: c, rnd: rnd, limit: limit, publicURL: publicURL, log: log}
}

// Execute membuat key baru. Plaintext hanya hidup di variabel lokal sampai return.
func (u *Usecase) Execute(ctx context.Context, projectID string, in CreateRequest, meta base.RequestMeta) (base.CreateKeyResult, error) {
	var zero base.CreateKeyResult
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return zero, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if err := u.access.Require(ctx, projectID, actor); err != nil {
		return zero, err
	}
	name, fe := base.ValidateName(in.Name)
	if len(fe) > 0 {
		return zero, base.InvalidFields(fe)
	}
	now := u.clock()

	var (
		created base.Key
		rawFull string
		done    bool
	)
	for attempt := 1; attempt <= maxPrefixAttempts; attempt++ {
		raw, err := base.GenerateRawKey(u.rnd)
		if err != nil {
			return zero, fmt.Errorf("createkey: generate key: %w", err)
		}
		key, err := base.NewKey(projectID, name, raw, actor, now)
		if err != nil {
			return zero, err
		}
		var got base.Key
		err = u.repo.InTx(ctx, func(tx Tx) error {
			if err := tx.LockOwnerKeys(ctx, projectID, actor.ID); err != nil {
				return fmt.Errorf("createkey: lock: %w", err)
			}
			n, err := tx.CountActive(ctx, projectID, actor.ID)
			if err != nil {
				return fmt.Errorf("createkey: count: %w", err)
			}
			if err := base.CheckActiveLimit(n, u.limit); err != nil {
				return err
			}
			got, err = tx.Insert(ctx, key)
			if err != nil {
				var e *errs.Error
				if errors.As(err, &e) || errors.Is(err, base.ErrPrefixCollision) {
					return err
				}
				return fmt.Errorf("createkey: insert: %w", err)
			}
			return nil
		})
		if errors.Is(err, base.ErrPrefixCollision) {
			u.log.WarnContext(ctx, "createkey: prefix collision, mengulang",
				"feature", "createkey", "project_id", projectID, "attempt", attempt)
			continue
		}
		if err != nil {
			return zero, err
		}
		created, rawFull, done = got, raw.Full, true
		break
	}
	if !done {
		return zero, fmt.Errorf("createkey: prefix collision setelah %d percobaan: %w", maxPrefixAttempts, base.ErrPrefixCollision)
	}

	u.audit.Record(ctx, audit.Event{
		Actor: actor, Action: record.ActionAPIKeyCreated, ProjectID: projectID, KeyID: created.ID,
		Args: map[string]any{"name": created.Name, "prefix": created.Prefix}, Meta: meta,
	})
	return base.CreateKeyResult{Key: created.ToDTO(), RawKey: rawFull, MCPURL: base.MCPURL(u.publicURL)}, nil
}
