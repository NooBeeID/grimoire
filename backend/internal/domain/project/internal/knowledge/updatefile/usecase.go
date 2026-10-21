// Package updatefile menyimpan isi baru satu file knowledge dengan optimistic concurrency (version).
package updatefile

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/domain/project/internal/knowledge/base/access"
	"grimoire/internal/domain/project/internal/knowledge/base/audit"
	"grimoire/lib/contracts/core/audit/record"
)

// Repository adalah akses data updatefile (sisi consumer).
type Repository interface {
	FindByPath(ctx context.Context, projectID, path string) (base.File, bool, error)
	UpdateIfVersion(ctx context.Context, projectID, path string, expectedHash []byte, f base.File) (base.File, int, bool, error)
}

type Input struct {
	ProjectID string
	Path      string  // mentah dari query
	Content   *string // nil = tidak dikirim
	Version   string
	Meta      base.RequestMeta
}

type Usecase struct {
	repo   Repository
	access access.Checker
	audit  audit.Recorder
	limits base.Limits
}

func NewUsecase(r Repository, a access.Checker, au audit.Recorder, l base.Limits) *Usecase {
	return &Usecase{repo: r, access: a, audit: au, limits: l}
}

func (u *Usecase) Execute(ctx context.Context, in Input) (base.FileDTO, error) {
	actor, err := u.access.Require(ctx, in.ProjectID)
	if err != nil {
		return base.FileDTO{}, err
	}
	path, r := base.NormalizePath(in.Path)
	if r != "" {
		return base.FileDTO{}, base.Invalid(r)
	}
	if in.Content == nil {
		return base.FileDTO{}, base.InvalidMsg(base.ReasonInvalidEncoding, "content wajib diisi (boleh string kosong)")
	}
	content, r := base.PrepareContent([]byte(*in.Content), u.limits.MaxFileBytes)
	if r == base.ReasonTooLarge {
		return base.FileDTO{}, base.TooLarge(r, u.limits.MaxFileBytes)
	}
	if r != "" {
		return base.FileDTO{}, base.Invalid(r)
	}
	if !base.ValidVersion(in.Version) {
		return base.FileDTO{}, base.Invalid(base.ReasonInvalidVersion)
	}
	expected, err := hex.DecodeString(in.Version)
	if err != nil {
		return base.FileDTO{}, base.Invalid(base.ReasonInvalidVersion)
	}

	// Jalur no-op terverifikasi (D12): isi baru identik dengan version yang dikirim klien.
	if bytes.Equal(base.Hash(content), expected) {
		cur, ok, err := u.repo.FindByPath(ctx, in.ProjectID, path)
		if err != nil {
			return base.FileDTO{}, fmt.Errorf("updatefile: cari file: %w", err)
		}
		if !ok {
			return base.FileDTO{}, base.ErrFileNotFound
		}
		if !bytes.Equal(cur.Hash, expected) {
			return base.FileDTO{}, base.VersionConflict(cur.ToConflict())
		}
		return cur.ToDTO(), nil
	}

	next := base.File{ProjectID: in.ProjectID, Path: path}.WithContent(content, actor)
	var last base.File
	for attempt := 0; attempt < 2; attempt++ {
		upd, before, found, err := u.repo.UpdateIfVersion(ctx, in.ProjectID, path, expected, next)
		if err != nil {
			return base.FileDTO{}, err
		}
		if found {
			u.audit.Record(ctx, audit.Event{
				Action: record.ActionKnowledgeFileUpdated, ProjectID: in.ProjectID, Actor: actor,
				TargetType: audit.TargetFile, Target: path,
				Args:   map[string]any{},
				Result: map[string]any{"bytes_before": before, "bytes": upd.SizeBytes},
				Meta:   in.Meta,
			})
			return upd.ToDTO(), nil
		}
		cur, ok, err := u.repo.FindByPath(ctx, in.ProjectID, path)
		if err != nil {
			return base.FileDTO{}, fmt.Errorf("updatefile: cari file: %w", err)
		}
		if !ok {
			return base.FileDTO{}, base.ErrFileNotFound
		}
		if !bytes.Equal(cur.Hash, expected) {
			return base.FileDTO{}, base.VersionConflict(cur.ToConflict())
		}
		last = cur // hash cocok lagi: balapan langka, ulangi sekali
	}
	return base.FileDTO{}, base.VersionConflict(last.ToConflict())
}
