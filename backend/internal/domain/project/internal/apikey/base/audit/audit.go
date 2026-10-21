// Package audit mengirim catatan audit module api-key lewat bus (core.audit.record).
// Fail-open: kegagalan hanya di-log. Label project dari get-project bersifat fail-soft.
package audit

import (
	"context"
	"log/slog"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/getproject"
)

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

type Event struct {
	Actor     base.Actor
	Action    string // record.ActionAPIKeyCreated | record.ActionAPIKeyRevoked
	ProjectID string
	KeyID     string         // Target
	Args      map[string]any // {name, prefix}; DILARANG memuat raw key/secret/hash
	Meta      base.RequestMeta
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

// Record mengambil label project (fail-soft) lalu mengirim audit (fail-open). Args tidak pernah di-log.
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	label := ""
	g, err := bus.CallTyped[getproject.Response](ctx, r.b, constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT,
		getproject.Request{ProjectID: ev.ProjectID})
	if err != nil {
		r.log.WarnContext(ctx, "apikey: get-project untuk label audit gagal", "project_id", ev.ProjectID, "error", err)
	} else if g.Found {
		label = g.Name
	}
	req := record.Request{
		ActorType: record.ActorUser, ActorID: ev.Actor.ID, ActorLabel: ev.Actor.Label,
		ProjectID: ev.ProjectID, ProjectLabel: label,
		Action: ev.Action, TargetType: base.TargetType, Target: ev.KeyID,
		Args: ev.Args, Status: record.StatusOK,
		IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx),
	}
	if _, err := bus.CallTyped[record.Response](ctx, r.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit record gagal", "action", ev.Action, "project_id", ev.ProjectID, "key_id", ev.KeyID, "error", err)
	}
}
