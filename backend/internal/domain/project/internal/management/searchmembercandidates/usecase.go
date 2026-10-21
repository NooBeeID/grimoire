// Package searchmembercandidates melayani GET /api/projects/{projectID}/member-candidates.
package searchmembercandidates

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/users"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/finddevelopers"
)

type repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (base.Project, string, bool, error)
	ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
}

type Input struct {
	ProjectID string
	RawQ      string
	RawLimit  string
}

type Usecase struct {
	repo  repository
	users users.Lookup
}

func NewUsecase(repo repository, u users.Lookup) *Usecase { return &Usecase{repo: repo, users: u} }

// Execute: otorisasi dulu, validasi param sesudahnya (tidak bocor lewat 400 vs 404).
func (u *Usecase) Execute(ctx context.Context, in Input) ([]base.CandidateDTO, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return nil, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	p, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
	if err != nil {
		return nil, fmt.Errorf("searchmembercandidates: find access: %w", err)
	}
	if !found {
		return nil, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermManageMembers); err != nil {
		return nil, err
	}
	qry, err := parseQuery(in.RawQ, in.RawLimit)
	if err != nil {
		return nil, err
	}
	if qry.Q == "" {
		return []base.CandidateDTO{}, nil
	}
	members, err := u.repo.ListMembers(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("searchmembercandidates: list members: %w", err)
	}
	isMember := make(map[string]struct{}, len(members))
	for _, m := range members {
		isMember[m.UserID] = struct{}{}
	}
	devs, err := u.users.FindDevelopers(ctx, qry.Q, finddevelopers.MaxLimit)
	if err != nil {
		return nil, fmt.Errorf("searchmembercandidates: find developers: %w", err)
	}
	out := make([]base.CandidateDTO, 0, qry.Limit)
	for _, d := range devs {
		if _, skip := isMember[d.ID]; skip {
			continue
		}
		out = append(out, base.CandidateDTO{ID: d.ID, Name: d.Name, Email: d.Email, InvitationPending: d.InvitationPending})
		if len(out) == qry.Limit {
			break
		}
	}
	return out, nil
}
