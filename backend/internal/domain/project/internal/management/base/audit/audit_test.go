package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

func logger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestRecord_BuildsRequest(t *testing.T) {
	log, _ := logger()
	b := bus.NewRegistry(log)
	var got record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		got = req.(record.Request)
		return record.Response{Recorded: true}, nil
	})
	ctx := reqctx.WithTraceID(reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Email: "u@x", Role: "developer"}), "tr1")
	New(b, log).Record(ctx, Event{
		Action: record.ActionProjectCreated, ProjectID: "p1", ProjectLabel: "Pay",
		TargetType: "project", Target: "p1", Args: map[string]any{"name": "Pay"},
		Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"},
	})
	if got.ActorType != record.ActorUser || got.ActorID != "u1" || got.ActorLabel != "u@x" ||
		got.ProjectID != "p1" || got.ProjectLabel != "Pay" || got.Status != record.StatusOK ||
		got.TraceID != "tr1" || got.IP != "1.2.3.4" || got.Action != record.ActionProjectCreated {
		t.Fatalf("%+v", got)
	}
}

func TestRecord_FailOpen(t *testing.T) {
	log, buf := logger()
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1"})
	// endpoint tidak terdaftar
	New(bus.NewRegistry(log), log).Record(ctx, Event{Action: "x"})
	if !strings.Contains(buf.String(), "audit record gagal") {
		t.Fatalf("log: %s", buf.String())
	}
	// handler error
	buf.Reset()
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) { return nil, errors.New("boom") })
	New(b, log).Record(ctx, Event{Action: "x"})
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("log: %s", buf.String())
	}
	// tanpa identitas
	buf.Reset()
	New(b, log).Record(context.Background(), Event{Action: "x"})
	if !strings.Contains(buf.String(), "identitas kosong") {
		t.Fatalf("log: %s", buf.String())
	}
}
