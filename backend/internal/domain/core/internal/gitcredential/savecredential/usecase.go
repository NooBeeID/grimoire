// Package savecredential menyimpan atau mengganti PAT milik user login untuk (provider, host).
package savecredential

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/base/audit"
	"grimoire/internal/infra/gitprovider"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/common/secret"
	"grimoire/lib/contracts/core/audit/record"
)

// VerifyFailure dikembalikan usecase bila provider menolak/gagal diverifikasi. Tidak memuat token atau URL.
type VerifyFailure struct {
	Provider base.Provider
	Host     string
	Cause    error
}

// Error konstan: tidak memuat Cause.Error().
func (e *VerifyFailure) Error() string { return "savecredential: verify failed" }
func (e *VerifyFailure) Unwrap() error { return e.Cause }

type Verifier interface {
	VerifyToken(ctx context.Context, p gitprovider.Provider, host string, token secret.String) error
}
type HostPolicy interface {
	Allowed(p gitprovider.Provider, host string) bool
}
type Sealer interface {
	Seal(plain, aad []byte) ([]byte, error)
}
type Repository interface {
	Upsert(ctx context.Context, c base.Credential) (base.Saved, error)
}

type Usecase struct {
	enabled  bool
	policy   HostPolicy
	verifier Verifier
	sealer   Sealer
	repo     Repository
	audit    audit.Recorder
	clock    base.Clock
	log      *slog.Logger
}

// NewUsecase merakit usecase. enabled => sealer wajib ada (panic saat wiring bila tidak).
func NewUsecase(enabled bool, p HostPolicy, v Verifier, s Sealer, r Repository, a audit.Recorder, c base.Clock, log *slog.Logger) *Usecase {
	if enabled && s == nil {
		panic("savecredential: sealer wajib saat fitur aktif")
	}
	if log == nil {
		log = slog.Default()
	}
	if c == nil {
		c = base.SystemClock
	}
	return &Usecase{enabled: enabled, policy: p, verifier: v, sealer: s, repo: r, audit: a, clock: c, log: log}
}

type Result struct {
	Provider  string
	Host      string
	Last4     string
	UpdatedAt time.Time
	Replaced  bool
}

// Execute menjalankan alur simpan; urutan langkah wajib, kegagalan di langkah mana pun menghentikan alur.
func (u *Usecase) Execute(ctx context.Context, rawProvider string, in SaveRequest, meta base.RequestMeta) (Result, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	if !u.enabled {
		return Result{}, base.ErrFeatureDisabled
	}
	p, err := base.ParseProvider(rawProvider)
	if err != nil {
		return Result{}, err
	}
	host, err := base.NormalizeHost(p, in.Host)
	if err != nil {
		return Result{}, err
	}
	if !u.policy.Allowed(gitprovider.Provider(p), host) {
		return Result{}, base.HostNotAllowed(truncate(host, 253))
	}
	token, err := base.ValidateToken(in.Token.Reveal())
	if err != nil {
		return Result{}, err
	}

	if err := u.verifier.VerifyToken(ctx, gitprovider.Provider(p), host, token); err != nil {
		u.logVerifyFailure(ctx, p, host, err)
		return Result{}, &VerifyFailure{Provider: p, Host: host, Cause: err}
	}

	enc, err := u.sealer.Seal([]byte(token.Reveal()), base.AAD(actor.ID, p, host))
	if err != nil {
		return Result{}, fmt.Errorf("savecredential: seal: %w", err)
	}
	saved, err := u.repo.Upsert(ctx, base.Credential{
		UserID: actor.ID, Provider: p, Host: host, TokenEnc: enc, Last4: token.Last4(),
	})
	if err != nil {
		return Result{}, err
	}

	u.audit.Record(ctx, audit.Event{
		Actor: actor, Action: record.ActionGitCredentialSaved, Provider: p, Host: host,
		Args: map[string]any{"provider": string(p), "host": host, "replaced": saved.Replaced},
		Meta: meta,
	})
	return Result{
		Provider: string(p), Host: host, Last4: token.Last4(),
		UpdatedAt: saved.UpdatedAt, Replaced: saved.Replaced,
	}, nil
}

// logVerifyFailure tidak pernah menulis err.Error() (cause bisa membawa konteks URL).
func (u *Usecase) logVerifyFailure(ctx context.Context, p base.Provider, host string, err error) {
	u.log.DebugContext(ctx, "git verify failed", "provider", string(p), "host", host)
	if errors.Is(err, gitprovider.ErrUnauthorized) || errors.Is(err, gitprovider.ErrForbidden) {
		return
	}
	kind := "other"
	switch {
	case errors.Is(err, gitprovider.ErrRateLimited):
		kind = "rate_limited"
	case errors.Is(err, gitprovider.ErrUpstream):
		kind = "upstream"
	}
	u.log.WarnContext(ctx, "git verify failed", "provider", string(p), "host", host, slog.String("cause_kind", kind))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
