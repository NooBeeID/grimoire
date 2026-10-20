package login

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/domain/core/internal/auth/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository didefinisikan di sisi consumer.
type Repository interface {
	FindUserByEmail(ctx context.Context, email string) (base.User, bool, error)
	GetThrottle(ctx context.Context, email string) (base.LoginThrottle, error)
	UpdateThrottle(ctx context.Context, email string, now time.Time, mutate func(*base.LoginThrottle)) (base.LoginThrottle, error)
	CompleteLogin(ctx context.Context, email, userID string, s base.Session, now time.Time) error
}

// Input adalah masukan usecase.
type Input struct {
	Email    string
	Password base.Secret
	Meta     base.RequestMeta
}

// Result adalah hasil sukses.
type Result struct {
	Token  string // hanya untuk cookie; tidak pernah di-log
	MaxAge int    // detik
	User   base.MeDTO
}

// Usecase mengorkestrasi login.
type Usecase struct {
	repo     Repository
	hasher   base.Hasher
	dummy    string
	clock    base.Clock
	rand     io.Reader
	session  base.SessionConfig
	throttle base.ThrottlePolicy
	audit    audit.Recorder
	log      *slog.Logger
}

// NewUsecase membuat Usecase.
func NewUsecase(repo Repository, s base.Shared, rec audit.Recorder, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{
		repo: repo, hasher: s.Hasher, dummy: s.DummyHash, clock: s.Clock, rand: s.Rand,
		session: s.Session, throttle: s.Throttle, audit: rec, log: log,
	}
}

// Execute memverifikasi kredensial dan membuat sesi.
func (u *Usecase) Execute(ctx context.Context, in Input) (Result, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	pw := in.Password.Reveal()
	fe := base.FieldErrors{}
	switch {
	case email == "":
		fe["email"] = "wajib diisi"
	case len(email) > base.MaxEmailBytes:
		fe["email"] = "maksimal 254 byte"
	}
	switch {
	case pw == "":
		fe["password"] = "wajib diisi"
	case len(pw) > base.MaxPasswordBytes:
		fe["password"] = "maksimal 256 byte"
	}
	if len(fe) > 0 {
		return Result{}, base.InvalidFields(fe)
	}

	now := u.clock()
	th, err := u.repo.GetThrottle(ctx, email)
	if err != nil {
		return Result{}, fmt.Errorf("login: baca throttle: %w", err)
	}
	if th.IsLocked(now) {
		u.audit.Record(ctx, failedEvent(email, nil, "locked", in.Meta))
		return Result{}, base.AccountLocked(th.RetryAfter(now))
	}

	user, found, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return Result{}, fmt.Errorf("login: cari user: %w", err)
	}
	hash := u.dummy
	if found {
		hash = user.PasswordHash
	}
	ok, verr := u.hasher.Verify(hash, pw)
	if verr != nil {
		u.log.ErrorContext(ctx, "login: verifikasi hash gagal", "user_id", user.ID, "error", verr)
		ok = false
	}

	if !(found && ok && user.CanLogin()) {
		reason := "invalid_credentials"
		if found && ok && !user.CanLogin() {
			reason = "inactive"
		}
		if _, err := u.repo.UpdateThrottle(ctx, email, now, func(t *base.LoginThrottle) {
			t.RegisterFailure(now, u.throttle)
		}); err != nil {
			return Result{}, fmt.Errorf("login: update throttle: %w", err)
		}
		var up *base.User
		if found {
			up = &user
		}
		u.audit.Record(ctx, failedEvent(email, up, reason, in.Meta))
		return Result{}, base.ErrInvalidCreds
	}

	tok, err := base.NewSessionToken(u.rand)
	if err != nil {
		return Result{}, fmt.Errorf("login: token: %w", err)
	}
	sess := base.NewSession(user.ID, base.HashToken(u.session.Secret, tok), now, u.session, in.Meta)
	if err := u.repo.CompleteLogin(ctx, email, user.ID, sess, now); err != nil {
		return Result{}, fmt.Errorf("login: selesaikan login: %w", err)
	}
	u.audit.Record(ctx, audit.Event{
		Action: "auth.login.success", TargetType: "user", Target: user.ID,
		Args: map[string]any{"email": user.Email}, Status: record.StatusOK, Meta: in.Meta,
		Actor: &audit.Actor{Type: record.ActorUser, ID: user.ID, Label: user.Email},
	})
	return Result{Token: tok, MaxAge: sess.MaxAgeSeconds(u.session), User: user.ToMe()}, nil
}

func failedEvent(email string, user *base.User, reason string, meta base.RequestMeta) audit.Event {
	ev := audit.Event{
		Action: "auth.login.failed", TargetType: "user",
		Args:   map[string]any{"email_attempted": email, "reason": reason},
		Status: record.StatusDenied, Meta: meta,
		Actor: &audit.Actor{Type: record.ActorAnonymous, Label: email},
	}
	if user != nil {
		ev.Target = user.ID
		ev.Actor = &audit.Actor{Type: record.ActorUser, ID: user.ID, Label: email}
	}
	return ev
}
