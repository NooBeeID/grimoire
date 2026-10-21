package inviteuser

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
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/core/auth/invitation"
)

// Repository adalah kebutuhan usecase terhadap penyimpanan (sisi consumer).
type Repository interface {
	FindTarget(ctx context.Context, email string) (Target, error)
	CountInvitedSince(ctx context.Context, inviterID string, since time.Time) (int, time.Time, error)
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Tx adalah operasi di dalam satu transaksi.
type Tx interface {
	InsertUser(ctx context.Context, u base.User) (base.User, error)
	InsertInvitation(ctx context.Context, inv base.Invitation) error
	LockUser(ctx context.Context, id string) (base.User, bool, error)
	LockInvitation(ctx context.Context, userID string) (base.Invitation, bool, error)
	UpdateInvitation(ctx context.Context, inv base.Invitation) error
	UpdatePassword(ctx context.Context, id, hash string, mustChange bool, now time.Time) error
	DeleteSessions(ctx context.Context, userID string) (int64, error)
}

// SendFunc mengirim email undangan dan mengembalikan base.Delivery*.
// Dirakit di init.go; usecase tidak meng-import infra/mailer.
type SendFunc func(ctx context.Context, to string, c base.EmailContent) string

// Usecase menerbitkan undangan developer (orkestrasi saja; rule ada di base).
type Usecase struct {
	repo     Repository
	hasher   base.Hasher
	audit    audit.Recorder
	clock    base.Clock
	rand     io.Reader
	policy   base.InvitationPolicy
	loginURL string
	send     SendFunc
	log      *slog.Logger
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, hasher base.Hasher, a audit.Recorder, clock base.Clock, rnd io.Reader,
	policy base.InvitationPolicy, loginURL string, send SendFunc) *Usecase {
	return &Usecase{repo: repo, hasher: hasher, audit: a, clock: clock, rand: rnd, policy: policy,
		loginURL: loginURL, send: send, log: slog.Default()}
}

// WithLogger mengganti logger (default slog.Default()).
func (u *Usecase) WithLogger(l *slog.Logger) *Usecase {
	if l != nil {
		u.log = l
	}
	return u
}

type credential struct {
	pw   base.Secret // plaintext sementara; tidak pernah dilog
	hash string
}

// newCredential membangkitkan password lalu menghash-nya.
func (u *Usecase) newCredential() (credential, error) {
	pw, err := base.GeneratePassword(u.rand)
	if err != nil {
		return credential{}, fmt.Errorf("inviteuser: generate password: %w", err)
	}
	hash, err := u.hasher.Hash(pw)
	if err != nil {
		return credential{}, fmt.Errorf("inviteuser: hash: %w", err)
	}
	return credential{pw: base.Secret(pw), hash: hash}, nil
}

// Execute menjalankan undangan untuk kedua titik masuk (HTTP dan bus).
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error) {
	if in.InvitedBy == "" {
		return Result{}, fmt.Errorf("inviteuser: InvitedBy kosong")
	}
	now := u.clock().Truncate(time.Microsecond)

	name := in.Name
	if email, err := base.NormalizeEmail(in.Email); err == nil && strings.TrimSpace(name) == "" {
		name = base.DefaultNameFromEmail(email)
	}
	user, fe := base.NewUser(base.NewUserInput{
		Email: in.Email, Name: name, Role: base.RoleDeveloper,
		CreatedBy: in.InvitedBy, CreatedByLabel: in.InvitedByLabel, Now: now,
	})
	if len(fe) > 0 {
		return Result{}, base.InvalidFields(fe)
	}

	tgt, err := u.repo.FindTarget(ctx, user.Email)
	if err != nil {
		return Result{}, fmt.Errorf("inviteuser: cari target: %w", err)
	}
	mode, err := base.CheckInviteTarget(tgt.User, tgt.Invitation)
	if err != nil {
		return Result{}, err
	}

	var (
		cred    *credential
		inv     base.Invitation
		created bool
		resent  bool
	)
	ensureCred := func() error {
		if cred != nil {
			return nil
		}
		c, err := u.newCredential()
		if err != nil {
			return err
		}
		cred = &c
		return nil
	}

	if mode == base.InviteCreateNew {
		n, oldest, err := u.repo.CountInvitedSince(ctx, in.InvitedBy, now.Add(-time.Hour))
		if err != nil {
			return Result{}, fmt.Errorf("inviteuser: hitung kuota: %w", err)
		}
		if err := base.CheckInviterQuota(n, oldest, now, u.policy); err != nil {
			return Result{}, err
		}
		if err := ensureCred(); err != nil {
			return Result{}, err
		}
		user.PasswordHash = cred.hash
		var insErr error
		user, inv, insErr = u.create(ctx, user, in, now)
		switch {
		case insErr == nil:
			created = true
		case errors.Is(insErr, base.ErrEmailTaken):
			tgt, err = u.repo.FindTarget(ctx, user.Email)
			if err != nil {
				return Result{}, fmt.Errorf("inviteuser: cari target: %w", err)
			}
			if mode, err = base.CheckInviteTarget(tgt.User, tgt.Invitation); err != nil {
				return Result{}, err
			}
			if mode != base.InviteReissuePending || tgt.User == nil {
				return Result{}, fmt.Errorf("inviteuser: email bentrok tapi user tidak ada")
			}
		default:
			return Result{}, fmt.Errorf("inviteuser: simpan undangan: %w", insErr)
		}
	}

	res := Result{Now: now, EmailStatus: invitation.EmailStatusPending}
	if !created {
		user = *tgt.User
		if !in.Deliver {
			if err := tgt.Invitation.CheckResend(now, u.policy); err != nil {
				return Result{}, err
			}
			inv = *tgt.Invitation
		} else {
			if err := ensureCred(); err != nil {
				return Result{}, err
			}
			user, inv, err = u.reissue(ctx, user.ID, cred.hash, now)
			if err != nil {
				return Result{}, err
			}
			resent = true
		}
	}

	var status string
	if in.Deliver {
		status, err = u.deliver(ctx, user, &inv, cred.pw, in, now)
		if err != nil {
			return Result{}, err
		}
		res.EmailStatus = status
		res.EmailSent = status == base.DeliverySent
		if !res.EmailSent {
			res.Credentials = &invitation.Credentials{Email: user.Email, TemporaryPassword: cred.pw.Reveal(), LoginURL: u.loginURL}
		}
	}
	res.User, res.Invitation, res.Created = user, inv, created

	u.recordAudit(ctx, in, user, inv, created, resent, in.Deliver, status)
	return res, nil
}

// create menyimpan akun dan undangan dalam satu tx.
func (u *Usecase) create(ctx context.Context, user base.User, in Input, now time.Time) (base.User, base.Invitation, error) {
	var inv base.Invitation
	var out base.User
	err := u.repo.InTx(ctx, func(tx Tx) error {
		c, err := tx.InsertUser(ctx, user)
		if err != nil {
			return err
		}
		inv = base.NewInvitation(c.ID, in.InvitedBy, in.InvitedByLabel, now, u.policy)
		if err := tx.InsertInvitation(ctx, inv); err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		return user, base.Invitation{}, err
	}
	return out, inv, nil
}

// reissue menerbitkan ulang akun menunggu (urutan lock: user lalu undangan).
func (u *Usecase) reissue(ctx context.Context, userID, hash string, now time.Time) (base.User, base.Invitation, error) {
	var user base.User
	var inv base.Invitation
	err := u.repo.InTx(ctx, func(tx Tx) error {
		lu, found, err := tx.LockUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("inviteuser: lock user: %w", err)
		}
		if !found {
			return base.ErrUserNotFound
		}
		li, found, err := tx.LockInvitation(ctx, userID)
		if err != nil {
			return fmt.Errorf("inviteuser: lock undangan: %w", err)
		}
		if !found {
			return base.ErrInvitationNotPending
		}
		mode, err := base.CheckInviteTarget(&lu, &li)
		if err != nil {
			return err
		}
		if mode != base.InviteReissuePending {
			return base.ErrInvitationNotPending
		}
		if err := li.RegisterResend(now, u.policy); err != nil {
			return err
		}
		li.Reissue(now, u.policy)
		if err := tx.UpdatePassword(ctx, userID, hash, true, now); err != nil {
			return fmt.Errorf("inviteuser: ganti password: %w", err)
		}
		if _, err := tx.DeleteSessions(ctx, userID); err != nil {
			return fmt.Errorf("inviteuser: hapus sesi: %w", err)
		}
		if err := tx.UpdateInvitation(ctx, li); err != nil {
			return fmt.Errorf("inviteuser: ubah undangan: %w", err)
		}
		lu.PasswordHash, lu.MustChangePassword, lu.UpdatedAt = hash, true, now
		user, inv = lu, li
		return nil
	})
	if err != nil {
		return base.User{}, base.Invitation{}, wrapUnlessErrs(err)
	}
	return user, inv, nil
}

// wrapUnlessErrs membiarkan error terstruktur (untuk HTTP) dan membungkus error infra.
func wrapUnlessErrs(err error) error {
	var ee *errs.Error
	if errors.As(err, &ee) {
		return err
	}
	return fmt.Errorf("inviteuser: tx terbitkan ulang: %w", err)
}

// deliver merender dan mengirim email lalu mencatat statusnya. Mengembalikan status kirim.
func (u *Usecase) deliver(ctx context.Context, user base.User, inv *base.Invitation, pw base.Secret, in Input, now time.Time) (string, error) {
	c, err := base.RenderInvitationEmail(base.InvitationEmailData{
		InviteeEmail: user.Email, InviterName: in.InvitedByLabel, ProjectName: in.ProjectName,
		LoginURL: u.loginURL, TemporaryPassword: pw.Reveal(), ExpiresAt: inv.ExpiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("inviteuser: render email: %w", err)
	}
	sendCtx := context.WithoutCancel(ctx)
	status := u.send(sendCtx, user.Email, c)
	issuedAt := inv.IssuedAt
	inv.MarkDelivery(status, now)
	recErr := u.repo.InTx(sendCtx, func(tx Tx) error {
		cur, found, err := tx.LockInvitation(sendCtx, user.ID)
		if err != nil {
			return err
		}
		if !found || !cur.IssuedAt.Equal(issuedAt) {
			return nil // sudah diterbitkan ulang oleh request lain
		}
		cur.MarkDelivery(status, now)
		return tx.UpdateInvitation(sendCtx, cur)
	})
	if recErr != nil {
		u.log.ErrorContext(sendCtx, "inviteuser: catat status kirim gagal", "user_id", user.ID, "status", status, "err", recErr)
	}
	return status, nil
}

func (u *Usecase) recordAudit(ctx context.Context, in Input, user base.User, inv base.Invitation, created, resent, deliver bool, status string) {
	args := func(extra map[string]any) map[string]any {
		m := map[string]any{"email": user.Email, "via": in.Via}
		if in.ProjectName != "" {
			m["project_name"] = in.ProjectName
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	u.audit.Record(ctx, audit.Event{
		Action: record.ActionUserInvited, TargetType: "user", Target: user.ID,
		Args: args(map[string]any{"created": created}), Status: record.StatusOK, Meta: in.Meta,
	})
	if resent && deliver {
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionUserInvitationResent, TargetType: "user", Target: user.ID,
			Args: args(map[string]any{"resend_count": inv.ResendCount}), Status: record.StatusOK, Meta: in.Meta,
		})
	}
	if deliver && status != base.DeliverySent {
		class := "send_failed"
		if status == base.DeliveryNotConfigured {
			class = "not_configured"
		}
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionUserInvitationEmailFailed, TargetType: "user", Target: user.ID,
			Args:   map[string]any{"email": user.Email, "delivery_status": status, "error_class": class},
			Status: record.StatusError, Meta: in.Meta,
		})
	}
}
