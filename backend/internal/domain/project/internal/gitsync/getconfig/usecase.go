package getconfig

import (
	"context"
	"fmt"
	"log/slog"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/domain/project/internal/gitsync/base/access"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/gitcredential/gettoken"
)

type ConfigReader interface {
	Get(ctx context.Context, projectID string, kind base.Kind) (*base.Config, error)
	InProgress(ctx context.Context, projectID string, kind base.Kind) (bool, error)
}

// CredentialStatus adalah status PAT; sengaja tanpa field token.
type CredentialStatus struct{ Found, Unreadable, Disabled bool }

// CredentialLookup membungkus bus get-token. Hanya status yang keluar.
type CredentialLookup interface {
	Status(ctx context.Context, userID, provider, host string) (CredentialStatus, error)
}

type GetResponse struct {
	Kind                 string          `json:"kind"`
	Config               *base.ConfigDTO `json:"config"`
	HasCredential        bool            `json:"has_credential"`
	CredentialUnreadable bool            `json:"credential_unreadable"`
	FeatureEnabled       bool            `json:"feature_enabled"`
	InProgress           bool            `json:"in_progress"`
}

type Usecase struct {
	access  access.Checker
	repo    ConfigReader
	cred    CredentialLookup
	enabled bool
	log     *slog.Logger
}

func NewUsecase(a access.Checker, r ConfigReader, c CredentialLookup, featureEnabled bool, log *slog.Logger) *Usecase {
	if log == nil {
		log = slog.Default()
	}
	return &Usecase{access: a, repo: r, cred: c, enabled: featureEnabled, log: log}
}

func (u *Usecase) Execute(ctx context.Context, in params) (GetResponse, error) {
	actor, err := u.access.Require(ctx, in.ProjectID)
	if err != nil {
		return GetResponse{}, err
	}
	kind, err := base.ParseKind(in.RawKind)
	if err != nil {
		return GetResponse{}, err
	}
	cfg, err := u.repo.Get(ctx, in.ProjectID, kind)
	if err != nil {
		return GetResponse{}, fmt.Errorf("getconfig: %w", err)
	}
	resp := GetResponse{Kind: string(kind), FeatureEnabled: u.enabled}
	resp.InProgress = u.inProgress(ctx, in.ProjectID, kind)
	if cfg == nil {
		return resp, nil
	}
	dto := cfg.ToDTO()
	if cfg.LastResult != nil {
		capped := cfg.LastResult.CapForStorage(base.MaxStoredReportItems).ToDTO()
		dto.LastResult = &capped
	}
	resp.Config = &dto

	if u.enabled {
		st, err := u.cred.Status(ctx, actor.ID, cfg.Provider, cfg.Host)
		switch {
		case err != nil:
			u.log.WarnContext(ctx, "gitsync getconfig: cek token gagal", "project_id", in.ProjectID, "kind", string(kind), "error", err)
		case st.Disabled:
			resp.FeatureEnabled = false
		default:
			resp.HasCredential = st.Found && !st.Unreadable
			resp.CredentialUnreadable = st.Found && st.Unreadable
		}
	}
	return resp, nil
}

// inProgress fail-soft: error cek lock -> false + log warn.
func (u *Usecase) inProgress(ctx context.Context, projectID string, kind base.Kind) bool {
	ok, err := u.repo.InProgress(ctx, projectID, kind)
	if err != nil {
		u.log.WarnContext(ctx, "gitsync getconfig: cek lock sync gagal", "project_id", projectID, "kind", string(kind), "error", err)
		return false
	}
	return ok
}

// busCredentials memanggil core.gitcredential.get-token; Token dibuang.
type busCredentials struct{ b *bus.Registry }

func (c busCredentials) Status(ctx context.Context, userID, provider, host string) (CredentialStatus, error) {
	res, err := bus.CallTyped[gettoken.Response](ctx, c.b, constants.BUS_CORE_GITCREDENTIAL_GET_TOKEN,
		gettoken.Request{UserID: userID, Provider: provider, Host: host})
	if err != nil {
		return CredentialStatus{}, err
	}
	return CredentialStatus{Found: res.Found, Unreadable: res.Unreadable, Disabled: res.Disabled}, nil
}
