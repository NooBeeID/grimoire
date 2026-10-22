// Package audit mengirim catatan audit git_sync.run lewat bus (core.audit.record), fail-open.
// Args/Result hanya metadata: tidak pernah token, URL lengkap dengan query, atau isi file.
package audit

import (
	"context"
	"log/slog"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/getproject"
)

// TargetType adalah jenis target audit; Target = kind.
const TargetType = "git_sync"

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

// Event: satu sync. ErrorCode terisi -> status audit error.
type Event struct {
	Actor        base.Actor
	ProjectID    string
	Kind         base.Kind
	Provider     string
	Host         string
	RepoPath     string
	Branch       string
	Folder       string
	TargetPrefix string
	Status       base.Status
	Summary      base.Summary
	ErrorCode    string
	Meta         base.RequestMeta
}

type BusRecorder struct {
	b   *bus.Registry
	log *slog.Logger
}

var _ Recorder = (*BusRecorder)(nil)

func New(b *bus.Registry, log *slog.Logger) *BusRecorder {
	if log == nil {
		log = slog.Default()
	}
	return &BusRecorder{b: b, log: log}
}

// Record mengambil label project (fail-soft) lalu mengirim audit (fail-open).
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	label := ""
	g, err := bus.CallTyped[getproject.Response](ctx, r.b, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT,
		getproject.Request{ProjectID: ev.ProjectID})
	if err != nil {
		r.log.WarnContext(ctx, "gitsync: get-project untuk label audit gagal", "project_id", ev.ProjectID, "error", err)
	} else if g.Found {
		label = g.Name
	}
	status := record.StatusOK
	if ev.ErrorCode != "" || ev.Status == base.StatusFailed {
		status = record.StatusError
	}
	req := record.Request{
		ActorType: record.ActorUser, ActorID: ev.Actor.ID, ActorLabel: ev.Actor.Label,
		ProjectID: ev.ProjectID, ProjectLabel: label,
		Action: record.ActionGitSyncRun, TargetType: TargetType, Target: string(ev.Kind),
		Args: map[string]any{
			"kind": string(ev.Kind), "provider": ev.Provider, "host": ev.Host, "repo_path": ev.RepoPath,
			"branch": ev.Branch, "folder": ev.Folder, "target_prefix": ev.TargetPrefix,
		},
		Result: map[string]any{
			"status": string(ev.Status), "created": ev.Summary.Created, "updated": ev.Summary.Updated,
			"unchanged": ev.Summary.Unchanged, "skipped": ev.Summary.Skipped, "rejected": ev.Summary.Rejected,
			"error_code": ev.ErrorCode,
		},
		Status: status,
		IP:     ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx),
	}
	if _, err := bus.CallTyped[record.Response](ctx, r.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit write failed", "action", record.ActionGitSyncRun, "project_id", ev.ProjectID, "err", err)
	}
}
