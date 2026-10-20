package resolvesession

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

var tokenLen = base64.RawURLEncoding.EncodedLen(base.TokenBytes)

// Usecase menerjemahkan token cookie menjadi identitas terbaru.
type Usecase struct {
	repo  Repository
	clock base.Clock
	cfg   base.SessionConfig
	log   *slog.Logger
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, clock base.Clock, cfg base.SessionConfig, log *slog.Logger) *Usecase {
	return &Usecase{repo: repo, clock: clock, cfg: cfg, log: log}
}

// Execute: Found=false (tanpa error) untuk token/sesi/user tidak valid; error hanya untuk kegagalan infra.
func (u *Usecase) Execute(ctx context.Context, req resolvesession.Request) (resolvesession.Response, error) {
	if len(req.Token) != tokenLen {
		return resolvesession.Response{}, nil
	}
	hash := base.HashToken(u.cfg.Secret, req.Token)
	sess, user, found, err := u.repo.FindSession(ctx, hash)
	if err != nil {
		return resolvesession.Response{}, fmt.Errorf("resolvesession: cari sesi: %w", err)
	}
	if !found {
		return resolvesession.Response{}, nil
	}
	now := u.clock()
	if !sess.IsValid(now, u.cfg.IdleTimeout) || !user.CanLogin() {
		return resolvesession.Response{}, nil
	}
	if sess.NeedsTouch(now) {
		if _, err := u.repo.TouchSession(ctx, sess.ID, now); err != nil {
			u.log.WarnContext(ctx, "touch session gagal", "error", err)
		}
	}
	return resolvesession.Response{
		Found: true, UserID: user.ID, Email: user.Email, Name: user.Name,
		Role: user.Role, MustChangePassword: user.MustChangePassword,
	}, nil
}
