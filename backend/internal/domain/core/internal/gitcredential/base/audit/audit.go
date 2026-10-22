// Package audit mengirim catatan audit module gitcredential lewat bus (core.audit.record), fail-open.
// Args hanya {provider, host[, replaced]}; token tidak pernah masuk.
package audit

import (
	"context"
	"log/slog"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

// TargetType adalah jenis target audit.
const TargetType = "git_credential"

// Recorder mencatat satu event audit. Tidak mengembalikan error (fail-open).
type Recorder interface {
	Record(ctx context.Context, ev Event)
}

// Event: Action = record.ActionGitCredentialSaved | ActionGitCredentialDeleted.
type Event struct {
	Actor    base.Actor
	Action   string
	Provider base.Provider
	Host     string
	Args     map[string]any // opsional; default {provider, host}
	Meta     base.RequestMeta
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

// Record mengirim audit; kegagalan hanya di-log (Args tidak pernah di-log).
func (r *BusRecorder) Record(ctx context.Context, ev Event) {
	args := map[string]any{"provider": string(ev.Provider), "host": ev.Host}
	for k, v := range ev.Args {
		switch k {
		case "provider", "host", "replaced":
			args[k] = v
		}
	}
	req := record.Request{
		ActorType: record.ActorUser, ActorID: ev.Actor.ID, ActorLabel: ev.Actor.Label,
		Action: ev.Action, TargetType: TargetType, Target: string(ev.Provider) + ":" + ev.Host,
		Args: args, Status: record.StatusOK,
		IP: ev.Meta.IP, UserAgent: ev.Meta.UserAgent, TraceID: reqctx.TraceID(ctx),
	}
	if _, err := bus.CallTyped[record.Response](ctx, r.b, constants.BUS_CORE_AUDIT_RECORD, req); err != nil {
		r.log.ErrorContext(ctx, "audit write failed", "action", ev.Action, "err", err)
	}
}
