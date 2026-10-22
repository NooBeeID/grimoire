// Package deletecredential adalah feature DELETE /api/me/git-credentials/{provider}?host=.
// Menghapus PAT milik sendiri; tidak memeriksa HostPolicy dan tidak butuh kunci enkripsi.
package deletecredential

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/audit"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

type repository interface {
	// Delete: DELETE ... WHERE user_id, provider, host RETURNING 1.
	// deleted=false bila tidak ada baris. userID bukan uuid -> (false, nil) tanpa query.
	Delete(ctx context.Context, userID string, p base.Provider, host string) (deleted bool, err error)
}

// Usecase mengorkestrasi penghapusan credential.
type Usecase struct {
	repo  repository
	audit audit.Recorder
}

// NewUsecase membuat Usecase.
func NewUsecase(repo repository, a audit.Recorder) *Usecase {
	return &Usecase{repo: repo, audit: a}
}

// Execute menghapus credential milik aktor. Tidak ada baris -> ErrCredentialNotFound.
func (u *Usecase) Execute(ctx context.Context, in Input) error {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return errs.New(errs.CodeUnauthorized, "perlu login")
	}
	p, err := base.ParseProvider(in.Provider)
	if err != nil {
		return err
	}
	host, err := base.NormalizeHost(p, in.Host)
	if err != nil {
		return err
	}
	deleted, err := u.repo.Delete(ctx, actor.ID, p, host)
	if err != nil {
		return fmt.Errorf("deletecredential: delete: %w", err)
	}
	if !deleted {
		return base.ErrCredentialNotFound
	}
	u.audit.Record(ctx, audit.Event{
		Actor: actor, Action: record.ActionGitCredentialDeleted,
		Provider: p, Host: host, Meta: in.Meta,
	})
	return nil
}
