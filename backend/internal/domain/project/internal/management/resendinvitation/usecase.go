// Package resendinvitation melayani POST /api/projects/{projectID}/invitations/{userID}/resend
// (kirim ulang undangan anggota yang belum menerima).
package resendinvitation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

// Sisi consumer. Repo konkret memenuhi Repository.
type Repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
}

// Users dipenuhi users.BusLookup.
type Users interface {
	Get(ctx context.Context, userID string) (getuser.Response, error)
	ResendInvitation(ctx context.Context, req resendinvitation.Request) (resendinvitation.Response, error)
}

type Input struct{ ProjectID, UserID string }

type Usecase struct {
	repo  Repository
	users Users
	log   *slog.Logger
}

func NewUsecase(repo Repository, u Users, log *slog.Logger) *Usecase {
	return &Usecase{repo: repo, users: u, log: log}
}

// Execute mengorkestrasi kirim ulang undangan lewat bus auth. Tanpa transaksi dan tanpa audit
// di management (auth yang mengaudit).
func (u *Usecase) Execute(ctx context.Context, in Input) (ResendResponse, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return ResendResponse{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	p, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
	if err != nil {
		return ResendResponse{}, fmt.Errorf("resendinvitation: cari akses: %w", err)
	}
	if !found {
		return ResendResponse{}, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermManageMembers); err != nil {
		return ResendResponse{}, err
	}
	targetID, err := uuid.Parse(in.UserID)
	if err != nil {
		return ResendResponse{}, base.ErrMemberNotFound
	}
	target := targetID.String()

	members, err := u.repo.ListMembers(ctx, p.ID)
	if err != nil {
		return ResendResponse{}, fmt.Errorf("resendinvitation: daftar anggota: %w", err)
	}
	if _, ok := base.FindMember(members, target); !ok {
		return ResendResponse{}, base.ErrMemberNotFound
	}

	res, err := u.users.Get(ctx, target)
	if err != nil {
		return ResendResponse{}, fmt.Errorf("resendinvitation: get-user: %w", err)
	}
	if !res.Found {
		return ResendResponse{}, base.ErrMemberNotFound
	}
	if res.Invitation == nil {
		return ResendResponse{}, base.ErrInvitationNotPending
	}

	out, err := u.users.ResendInvitation(ctx, resendinvitation.Request{
		UserID: target, InvitedBy: actor.ID, InvitedByLabel: actor.Label,
		ProjectName: p.Name, Initial: false,
	})
	if err != nil {
		var ee *errs.Error
		if errors.As(err, &ee) {
			if ee.Code == errs.CodeNotFound {
				return ResendResponse{}, base.ErrMemberNotFound
			}
			return ResendResponse{}, err
		}
		return ResendResponse{}, fmt.Errorf("resendinvitation: kirim ulang: %w", err)
	}

	resp := ResendResponse{EmailSent: out.EmailSent, EmailStatus: out.EmailStatus, ExpiresAt: out.ExpiresAt.UTC()}
	if out.Credentials != nil {
		if out.EmailSent {
			u.log.WarnContext(ctx, "resendinvitation: kredensial dibuang karena email terkirim",
				"project_id", p.ID, "user_id", target)
		} else {
			resp.Credentials = &CredentialsResponse{
				Email:             out.Credentials.Email,
				TemporaryPassword: out.Credentials.TemporaryPassword,
				LoginURL:          out.Credentials.LoginURL,
			}
		}
	}
	return resp, nil
}
