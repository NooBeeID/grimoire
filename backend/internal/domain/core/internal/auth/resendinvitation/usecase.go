package resendinvitation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/internal/domain/core/internal/auth/base/delivery"
	"grimoire/internal/infra/mailer"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah kebutuhan usecase terhadap penyimpanan (sisi consumer).
type Repository interface {
	Issue(ctx context.Context, in IssueInput) (Issued, error)
	MarkDelivery(ctx context.Context, userID, status string, now time.Time) error
}

// Usecase menerbitkan ulang password sementara undangan (orkestrasi; rule di base).
type Usecase struct {
	repo     Repository
	hasher   base.Hasher
	mailer   mailer.Mailer
	audit    audit.Recorder
	clock    base.Clock
	rand     io.Reader
	policy   base.InvitationPolicy
	loginURL string
	mailTO   time.Duration
	log      *slog.Logger
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, hasher base.Hasher, ml mailer.Mailer, a audit.Recorder, clock base.Clock,
	rnd io.Reader, policy base.InvitationPolicy, loginURL string, mailTimeout time.Duration, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{repo: repo, hasher: hasher, mailer: ml, audit: a, clock: clock, rand: rnd,
		policy: policy, loginURL: loginURL, mailTO: mailTimeout, log: log}
}

// checkResendTarget memastikan user dan undangan boleh diterbitkan ulang.
// inv nil atau sudah diterima -> ErrInvitationNotPending; user nonaktif -> ALREADY_REGISTERED inactive.
// (Rule ini lokal di folder usecase karena base/ tidak boleh diubah oleh tahap ini.)
func checkResendTarget(u base.User, inv *base.Invitation) error {
	if inv == nil || !inv.IsPending() {
		return base.ErrInvitationNotPending
	}
	if !u.IsActive {
		return base.ErrAlreadyRegistered(base.AlreadyRegisteredInactive)
	}
	return nil
}

// Execute menjalankan penerbitan ulang untuk kedua titik masuk (HTTP dan bus).
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error) {
	if !validUUID(in.UserID) {
		return Result{}, base.ErrUserNotFound
	}
	pwStr, err := base.GeneratePassword(u.rand)
	if err != nil {
		return Result{}, fmt.Errorf("resendinvitation: generate password: %w", err)
	}
	pw := base.Secret(pwStr)
	hash, err := u.hasher.Hash(pw.Reveal())
	if err != nil {
		return Result{}, fmt.Errorf("resendinvitation: hash: %w", err)
	}
	now := u.clock().Truncate(time.Microsecond)

	issued, err := u.repo.Issue(ctx, IssueInput{
		UserID: in.UserID, Hash: hash, Now: now,
		Apply: func(user base.User, inv *base.Invitation) error {
			if err := checkResendTarget(user, inv); err != nil {
				return err
			}
			if !in.Initial {
				if err := inv.RegisterResend(now, u.policy); err != nil {
					return err
				}
			}
			inv.Reissue(now, u.policy)
			return nil
		},
	})
	if err != nil {
		var ee *errs.Error
		if errors.As(err, &ee) {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("resendinvitation: terbitkan: %w", err)
	}

	user, inv := issued.User, issued.Invitation
	status := u.deliver(ctx, user, inv, pw, in)

	markCtx := context.WithoutCancel(ctx)
	if err := u.repo.MarkDelivery(markCtx, user.ID, status, u.clock().Truncate(time.Microsecond)); err != nil {
		u.log.ErrorContext(markCtx, "resendinvitation: catat status kirim gagal", "user_id", user.ID, "status", status, "err", err)
	}

	u.recordAudit(ctx, in, user, inv, status)

	res := Result{EmailSent: status == base.DeliverySent, EmailStatus: status, ExpiresAt: inv.ExpiresAt}
	if !res.EmailSent {
		res.Credentials = &Credentials{Email: user.Email, TemporaryPassword: pw.Reveal(), LoginURL: u.loginURL}
	}
	return res, nil
}

// deliver merender dan mengirim email setelah commit. Render gagal dianggap failed.
func (u *Usecase) deliver(ctx context.Context, user base.User, inv base.Invitation, pw base.Secret, in Input) string {
	content, err := base.RenderInvitationEmail(base.InvitationEmailData{
		InviteeEmail: user.Email, InviterName: inviterLabel(ctx, in, inv), ProjectName: in.ProjectName,
		LoginURL: u.loginURL, TemporaryPassword: pw.Reveal(), ExpiresAt: inv.ExpiresAt,
	})
	if err != nil {
		u.log.ErrorContext(ctx, "resendinvitation: render email gagal", "user_id", user.ID, "err", err)
		return base.DeliveryFailed
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), u.mailTimeout())
	defer cancel()
	return delivery.Deliver(sendCtx, u.mailer, delivery.Message(user.Email, content), u.log)
}

func (u *Usecase) mailTimeout() time.Duration {
	if u.mailTO <= 0 {
		return 10 * time.Second
	}
	return u.mailTO
}

// inviterLabel: InvitedByLabel > nama identitas > email identitas > label tersimpan > "".
func inviterLabel(ctx context.Context, in Input, inv base.Invitation) string {
	if l := strings.TrimSpace(in.InvitedByLabel); l != "" {
		return l
	}
	if id, ok := reqctx.IdentityFrom(ctx); ok {
		if n := strings.TrimSpace(id.Name); n != "" {
			return n
		}
		if e := strings.TrimSpace(id.Email); e != "" {
			return e
		}
	}
	if inv.InvitedByLabel != nil {
		return strings.TrimSpace(*inv.InvitedByLabel)
	}
	return ""
}

func (u *Usecase) recordAudit(ctx context.Context, in Input, user base.User, inv base.Invitation, status string) {
	var actor *audit.Actor
	if _, ok := reqctx.IdentityFrom(ctx); !ok && in.InvitedBy != "" {
		actor = &audit.Actor{Type: record.ActorUser, ID: in.InvitedBy, Label: in.InvitedByLabel}
	}
	if !in.Initial {
		args := map[string]any{"email": user.Email, "via": in.Via, "resend_count": inv.ResendCount}
		if in.ProjectName != "" {
			args["project_name"] = in.ProjectName
		}
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionUserInvitationResent, TargetType: "user", Target: user.ID,
			Args: args, Status: record.StatusOK, Meta: in.Meta, Actor: actor,
		})
	}
	if status != base.DeliverySent {
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionUserInvitationEmailFailed, TargetType: "user", Target: user.ID,
			Args:   map[string]any{"email": user.Email, "delivery_status": status},
			Status: record.StatusError, Meta: in.Meta, Actor: actor,
		})
	}
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
		default:
			return false
		}
	}
	return true
}
