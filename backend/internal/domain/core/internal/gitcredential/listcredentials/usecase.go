// Package listcredentials melayani GET /api/me/git-credentials: status PAT milik user sendiri.
package listcredentials

import (
	"context"
	"fmt"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Lister membaca kredensial milik satu user. Tidak pernah mengembalikan token terenkripsi.
type Lister interface {
	ListByUser(ctx context.Context, userID string) ([]base.Credential, error)
}

// Settings adalah nilai config yang dihitung sekali saat Init.
type Settings struct {
	Enabled     bool
	GitLabHosts []string
}

// Result adalah body 200. Tidak ada field token.
type Result struct {
	Enabled            bool                 `json:"enabled"`
	AllowedGitLabHosts []string             `json:"allowed_gitlab_hosts"`
	Items              []base.CredentialDTO `json:"items"`
}

// Usecase mengorkestrasi daftar kredensial.
type Usecase struct {
	repo Lister
	set  Settings
}

// NewUsecase menyalin s.GitLabHosts (hindari aliasing); nil menjadi slice kosong.
func NewUsecase(r Lister, s Settings) *Usecase {
	hosts := make([]string, len(s.GitLabHosts))
	copy(hosts, s.GitLabHosts)
	s.GitLabHosts = hosts
	return &Usecase{repo: r, set: s}
}

// Execute mengembalikan kredensial milik user yang login.
func (u *Usecase) Execute(ctx context.Context) (Result, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return Result{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	creds, err := u.repo.ListByUser(ctx, actor.ID)
	if err != nil {
		return Result{}, fmt.Errorf("listcredentials: list: %w", err)
	}
	items := make([]base.CredentialDTO, 0, len(creds))
	for _, c := range creds {
		items = append(items, c.ToDTO())
	}
	hosts := make([]string, len(u.set.GitLabHosts))
	copy(hosts, u.set.GitLabHosts)
	return Result{Enabled: u.set.Enabled, AllowedGitLabHosts: hosts, Items: items}, nil
}
