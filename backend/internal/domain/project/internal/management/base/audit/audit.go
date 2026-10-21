// Package audit mengirim catatan audit module management lewat bus (core.audit.record).
// Fail-open: kegagalan hanya di-log, tidak pernah menggagalkan request.
package audit

import (
	"context"
	"log/slog"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

type Event struct {
	Action       string
	ProjectID    string
	ProjectLabel string         // snapshot nama project
	TargetType   string         // "project" | "project_member"
	Target       string         // project id atau user id target
	Args         map[string]any // TIDAK pernah memuat data sensitif
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

// Record membangun record.Request dari identitas di ctx lalu memanggil bus. Error hanya di-log.
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	id, ok := reqctx.IdentityFrom(ctx)
	if !ok || id.UserID == "" {
		r.log.ErrorContext(ctx, "audit record gagal: identitas kosong", "action", ev.Action, "project_id", ev.ProjectID)
		return
	}
	req := record.Request{
		ActorType: record.ActorUser, ActorID: id.UserID, ActorLabel: id.Email,
		ProjectID: ev.ProjectID, ProjectLabel: ev.ProjectLabel,
		Action: ev.Action, TargetType: ev.TargetType, Target: ev.Target,
		Args: ev.Args, Status: record.StatusOK,
		IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx),
	}
	if _, err := bus.CallTyped[record.Response](ctx, r.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "project_id", ev.ProjectID, "error", err)
	}
}
