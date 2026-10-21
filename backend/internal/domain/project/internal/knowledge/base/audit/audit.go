// Package audit mengirim catatan audit module knowledge lewat bus (core.audit.record), SETELAH commit.
// Fail-open: kegagalan hanya di-log. Label project dari get-project bersifat fail-soft.
// Args/Result hanya metadata; isi file tidak boleh masuk (rule 10.3, 13.6).
package audit

import (
	"context"
	"log/slog"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/getproject"
)

const (
	TargetFile      = "file"
	TargetFolder    = "folder"
	TargetKnowledge = "knowledge"
)

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

type Event struct {
	Action     string // record.ActionKnowledge*
	ProjectID  string
	Actor      base.Actor
	TargetType string // TargetFile | TargetFolder | TargetKnowledge
	Target     string
	Args       map[string]any
	Result     map[string]any
	Meta       base.RequestMeta
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

// Record mengambil label project (fail-soft) lalu mengirim audit (fail-open). Args/Result tidak pernah di-log.
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	label := ""
	g, err := bus.CallTyped[getproject.Response](ctx, r.b, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT,
		getproject.Request{ProjectID: ev.ProjectID})
	if err != nil {
		r.log.WarnContext(ctx, "knowledge: get-project untuk label audit gagal", "project_id", ev.ProjectID, "error", err)
	} else if g.Found {
		label = g.Name
	}
	req := record.Request{
		ActorType: record.ActorUser, ActorID: ev.Actor.ID, ActorLabel: ev.Actor.Label,
		ProjectID: ev.ProjectID, ProjectLabel: label,
		Action: ev.Action, TargetType: ev.TargetType, Target: ev.Target,
		Args: ev.Args, Result: ev.Result, Status: record.StatusOK,
		IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx),
	}
	if _, err := bus.CallTyped[record.Response](ctx, r.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit write failed", "action", ev.Action, "project_id", ev.ProjectID, "err", err)
	}
}
