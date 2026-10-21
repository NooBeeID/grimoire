// Package updateproject melayani PATCH /api/projects/{projectID} (ubah nama/deskripsi).
package updateproject

import (
	"context"
	"fmt"
	"time"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/domain/project/internal/management/base/audit"
	"grimoire/internal/domain/project/internal/management/base/counts"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah operasi di luar transaksi (sisi consumer).
type Repository interface {
	FindAccess(ctx context.Context, projectID, userID string) (p base.Project, memberRole string, found bool, err error)
	ListMembers(ctx context.Context, projectID string) ([]base.Member, error)
	InTx(ctx context.Context, fn func(TxRepository) error) error
}

// TxRepository adalah operasi di dalam satu transaksi.
type TxRepository interface {
	LockProject(ctx context.Context, id string) (base.Project, bool, error)
	UpdateProject(ctx context.Context, p base.Project) (updatedAt time.Time, err error)
}

type Input struct {
	ProjectID string
	Patch     base.ProjectPatch
	Meta      base.RequestMeta
}

type Usecase struct {
	repo   Repository
	audit  audit.Recorder
	counts counts.Counter
	clock  base.Clock
}

func NewUsecase(repo Repository, a audit.Recorder, c counts.Counter, clock base.Clock) *Usecase {
	return &Usecase{repo: repo, audit: a, counts: c, clock: clock}
}

// Execute mengubah project. No-op (nilai sama) tidak menulis dan tidak mengaudit.
func (u *Usecase) Execute(ctx context.Context, in Input) (base.ProjectDetail, error) {
	actor, ok := base.ActorFrom(reqctx.IdentityFrom(ctx))
	if !ok {
		return base.ProjectDetail{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	_, memberRole, found, err := u.repo.FindAccess(ctx, in.ProjectID, actor.ID)
	if err != nil {
		return base.ProjectDetail{}, fmt.Errorf("updateproject: cari akses: %w", err)
	}
	if !found {
		return base.ProjectDetail{}, base.ErrProjectNotFound
	}
	level := base.ResolveLevel(actor.Role, memberRole, true)
	if err := base.Require(level, base.PermEdit); err != nil {
		return base.ProjectDetail{}, err
	}

	var (
		updated base.Project
		changed []string
		oldName string
	)
	err = u.repo.InTx(ctx, func(tx TxRepository) error {
		p, ok, err := tx.LockProject(ctx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("updateproject: kunci project: %w", err)
		}
		if !ok {
			return base.ErrProjectNotFound
		}
		oldName = p.Name
		changed, err = p.ApplyUpdate(in.Patch, u.clock())
		if err != nil {
			return err
		}
		if len(changed) > 0 {
			at, err := tx.UpdateProject(ctx, p)
			if err != nil {
				return fmt.Errorf("updateproject: simpan: %w", err)
			}
			p.UpdatedAt = at
		}
		updated = p
		return nil
	})
	if err != nil {
		return base.ProjectDetail{}, err
	}

	if len(changed) > 0 {
		u.audit.Record(ctx, audit.Event{
			Action: record.ActionProjectUpdated, ProjectID: updated.ID, ProjectLabel: updated.Name,
			TargetType: "project", Target: updated.ID, Args: updateArgs(changed, oldName, updated.Name), Meta: in.Meta,
		})
	}

	members, err := u.repo.ListMembers(ctx, updated.ID)
	if err != nil {
		return base.ProjectDetail{}, fmt.Errorf("updateproject: daftar anggota: %w", err)
	}
	cs := u.counts.ForProjects(ctx, []string{updated.ID})[updated.ID]
	sum := base.NewSummary(updated, level, memberRole, len(members), cs)
	return base.ProjectDetail{ProjectSummary: sum, OwnerCount: base.OwnerCount(members)}, nil
}

// updateArgs merakit payload audit; deskripsi tidak pernah disertakan.
func updateArgs(changed []string, oldName, newName string) map[string]any {
	args := map[string]any{"changed_fields": changed}
	for _, f := range changed {
		if f == "name" {
			args["name_from"] = oldName
			args["name_to"] = newName
		}
	}
	return args
}
