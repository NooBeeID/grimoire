package logout

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/common/reqctx"
)

// Usecase mencabut sesi saat ini.
type Usecase struct {
	repo    Repository
	audit   audit.Recorder
	session base.SessionConfig
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, rec audit.Recorder, cfg base.SessionConfig) *Usecase {
	return &Usecase{repo: repo, audit: rec, session: cfg}
}

// Logout menghapus sesi berdasar hash token. Idempoten; audit hanya bila ada baris terhapus.
func (u *Usecase) Logout(ctx context.Context, p Params) error {
	if p.Token == "" {
		return nil
	}
	h := base.HashToken(u.session.Secret, p.Token)
	n, err := u.repo.DeleteSessionByHash(ctx, h)
	if err != nil {
		return fmt.Errorf("logout: hapus sesi: %w", err)
	}
	if n == 0 {
		return nil
	}
	id, _ := reqctx.IdentityFrom(ctx)
	u.audit.Record(ctx, audit.Event{
		Action: "auth.logout", TargetType: "user", Target: id.UserID,
		Args: map[string]any{}, Status: "ok", Meta: p.Meta,
	})
	return nil
}
