// Package inviteuser melayani POST /api/projects/{projectID}/invitations.
package inviteuser

import (
	"context"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/invitation"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

type Tx interface {
	LockProject(ctx context.Context, id string) (base.Project, bool, error)
	LockMembers(ctx context.Context, projectID string) ([]base.Member, error)
	InsertMember(ctx context.Context, m base.Member) (inserted bool, err error) // ON CONFLICT DO NOTHING
}

type Repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Usecase struct {
	repo  Repository
	users users.Lookup
	audit audit.Recorder
	clock base.Clock
	log   *slog.Logger
}

func NewUsecase(r Repository, u users.Lookup, a audit.Recorder, c base.Clock, l *slog.Logger) *Usecase {
	if l == nil {
		l = slog.Default()
	}
	return &Usecase{repo: r, users: u, audit: a, clock: c, log: l}
}

// Execute mengurutkan tiga langkah lintas module: siapkan akun (bus), tambah anggota (tx),
// terbitkan dan kirim (bus). Tidak ada panggilan bus di dalam tx; audit setelah commit.
func (u *Usecase) Execute(ctx context.Context, projectID string, in InviteRequest, meta base.RequestMeta) (InviteResponse, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return InviteResponse{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	project, memberRole, found, err := u.repo.FindAccess(ctx, projectID, actor.ID)
	if err != nil {
		return InviteResponse{}, fmt.Errorf("inviteuser: find access: %w", err)
	}
	if !found {
		return InviteResponse{}, base.ErrProjectNotFound
	}
	if err := base.Require(base.ResolveLevel(actor.Role, memberRole, true), base.PermManageMembers); err != nil {
		return InviteResponse{}, err
	}

	draft, err := base.NewMember(projectID, "", in.Role, actor, u.clock())
	if err != nil {
		return InviteResponse{}, err
	}

	email := normalizeEmail(in.Email)
	existing, err := u.users.GetByEmail(ctx, email)
	if err != nil {
		return InviteResponse{}, fmt.Errorf("inviteuser: get user by email: %w", err)
	}
	if existing.Found {
		_, targetRole, tfound, err := u.repo.FindAccess(ctx, projectID, existing.ID)
		if err != nil {
			return InviteResponse{}, fmt.Errorf("inviteuser: find target access: %w", err)
		}
		if !tfound {
			return InviteResponse{}, base.ErrProjectNotFound
		}
		if targetRole != "" {
			return InviteResponse{}, base.ErrAlreadyMember
		}
	}

	invited, err := u.users.Invite(ctx, inviteuser.Request{
		Email: email, Name: in.Name,
		InvitedBy: actor.ID, InvitedByLabel: actor.Label,
		ProjectName: project.Name, Deliver: false,
	})
	if err != nil {
		return InviteResponse{}, fmt.Errorf("inviteuser: invite: %w", err)
	}
	draft.UserID = invited.UserID

	var locked base.Project
	err = u.repo.InTx(ctx, func(tx Tx) error {
		p, ok, err := tx.LockProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("inviteuser: lock project: %w", err)
		}
		if !ok {
			return base.ErrProjectNotFound
		}
		locked = p
		ms, err := tx.LockMembers(ctx, projectID)
		if err != nil {
			return fmt.Errorf("inviteuser: lock members: %w", err)
		}
		mine, _ := base.FindMember(ms, actor.ID)
		if err := base.Require(base.ResolveLevel(actor.Role, mine.Role, true), base.PermManageMembers); err != nil {
			return err
		}
		if _, exists := base.FindMember(ms, draft.UserID); exists {
			return base.ErrAlreadyMember
		}
		inserted, err := tx.InsertMember(ctx, draft)
		if err != nil {
			return fmt.Errorf("inviteuser: insert member: %w", err)
		}
		if !inserted {
			return base.ErrAlreadyMember
		}
		return nil
	})
	if err != nil {
		return InviteResponse{}, err
	}

	emailSent, emailStatus, expiresAt := false, invitation.EmailStatusFailed, invited.ExpiresAt
	var creds *CredentialsDTO
	sent, rerr := u.users.ResendInvitation(ctx, resendinvitation.Request{
		UserID: invited.UserID, InvitedBy: actor.ID, InvitedByLabel: actor.Label,
		ProjectName: locked.Name, Initial: invited.Created,
	})
	if rerr != nil {
		u.log.ErrorContext(ctx, "invite: terbit/kirim gagal setelah anggota tercatat",
			"project_id", projectID, "user_id", invited.UserID, "err", rerr)
	} else {
		emailSent, emailStatus, expiresAt = sent.EmailSent, sent.EmailStatus, sent.ExpiresAt
		if !sent.EmailSent && sent.Credentials != nil {
			creds = &CredentialsDTO{
				Email: sent.Credentials.Email, TemporaryPassword: sent.Credentials.TemporaryPassword,
				LoginURL: sent.Credentials.LoginURL,
			}
		}
	}

	shown, gerr := u.users.Get(ctx, invited.UserID)
	if gerr != nil || !shown.Found {
		u.log.WarnContext(ctx, "invite: get-user gagal, memakai data fallback",
			"project_id", projectID, "user_id", invited.UserID, "err", gerr)
		shown = getuser.Response{
			Found: true, ID: invited.UserID, Email: email, Name: in.Name,
			Role: resolvesession.RoleDeveloper, IsActive: true,
		}
	}
	dto := draft.ToDTO(shown)
	if dto.Name != nil && *dto.Name == "" {
		dto.Name = nil
	}
	exp := expiresAt.UTC()
	dto.InvitationPending, dto.InvitationExpiresAt = true, &exp

	u.audit.Record(ctx, audit.Event{
		Action: record.ActionProjectMemberInvited, ProjectID: locked.ID, ProjectLabel: locked.Name,
		TargetType: "project_member", Target: draft.UserID,
		Args: map[string]any{"user_id": draft.UserID, "user_label": email, "role": draft.Role, "created": invited.Created},
		Meta: meta,
	})

	return InviteResponse{
		Member: dto, Created: invited.Created, EmailSent: emailSent, EmailStatus: emailStatus,
		ExpiresAt: exp, Credentials: creds,
	}, nil
}
