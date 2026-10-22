package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

var fixed = time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)

func setup(h bus.Handler) (*BusRecorder, *bytes.Buffer) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := bus.NewRegistry(log)
	if h != nil {
		r.Register(constants.BUS_CORE_AUDIT_RECORD, h)
	}
	return New(r, log, func() time.Time { return fixed }), &buf
}

func userCtx(name string) context.Context {
	ctx := reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Name: name, Email: "a@b", Role: "developer"})
	return reqctx.WithTraceID(ctx, "trace1")
}

func TestRecord_RequestShape(t *testing.T) {
	var got record.Request
	rec, _ := setup(func(ctx context.Context, req any) (any, error) {
		got = req.(record.Request)
		return record.Response{Recorded: true}, nil
	})
	rec.Record(userCtx("Budi"), Event{Action: record.ActionAPISpecUploaded, ProjectID: "p1", Target: "payment",
		Args: map[string]any{"name": "payment", "endpoint_count": 3}, Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"}})
	if got.ActorType != record.ActorUser || got.ActorID != "u1" || got.ActorLabel != "Budi" || got.TargetType != "api_spec" ||
		got.Target != "payment" || got.Action != record.ActionAPISpecUploaded || got.ProjectID != "p1" || got.ProjectLabel != "" ||
		got.Status != record.StatusOK || got.IP != "1.2.3.4" || got.UserAgent != "ua" || got.TraceID != "trace1" || !got.OccurredAt.Equal(fixed) {
		t.Fatalf("%+v", got)
	}
	if _, has := got.Args["content"]; has || got.Args["endpoint_count"] != 3 {
		t.Fatalf("%v", got.Args)
	}
}

func TestRecord_LabelFallsBackToEmail(t *testing.T) {
	var got record.Request
	rec, _ := setup(func(ctx context.Context, req any) (any, error) {
		got = req.(record.Request)
		return record.Response{}, nil
	})
	rec.Record(userCtx(""), Event{Action: record.ActionAPISpecDeleted, ProjectID: "p"})
	if got.ActorLabel != "a@b" {
		t.Fatalf("%q", got.ActorLabel)
	}
}

func TestRecord_FailOpen(t *testing.T) {
	rec, buf := setup(func(ctx context.Context, req any) (any, error) { return nil, errors.New("db down") })
	rec.Record(userCtx("B"), Event{Action: record.ActionAPISpecUploaded, ProjectID: "p1", Args: map[string]any{"secret": "JANGAN-DI-LOG"}})
	out := buf.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "audit record gagal") || strings.Contains(out, "JANGAN-DI-LOG") {
		t.Fatalf("log: %s", out)
	}
	// tidak terdaftar -> juga tidak panik.
	rec2, buf2 := setup(nil)
	rec2.Record(userCtx("B"), Event{Action: record.ActionAPISpecUpdated, ProjectID: "p1"})
	if !strings.Contains(buf2.String(), "level=ERROR") {
		t.Fatal("harus log error")
	}
}

func TestRecord_NoIdentityNotSent(t *testing.T) {
	called := false
	rec, buf := setup(func(ctx context.Context, req any) (any, error) { called = true; return record.Response{}, nil })
	rec.Record(context.Background(), Event{Action: record.ActionAPISpecDeleted, ProjectID: "p"})
	if called || !strings.Contains(buf.String(), "level=ERROR") {
		t.Fatalf("called=%v log=%s", called, buf.String())
	}
}

func TestRecord_ExplicitActorOverridesContext(t *testing.T) {
	var got record.Request
	rec, _ := setup(func(ctx context.Context, req any) (any, error) {
		got = req.(record.Request)
		return record.Response{Recorded: true}, nil
	})
	// ctx tanpa identitas (handler bus): Actor eksplisit dipakai.
	rec.Record(context.Background(), Event{Action: record.ActionAPISpecUploaded, ProjectID: "p1", Target: "a",
		Actor: &base.Actor{ID: "u9", Label: "Rina"}})
	if got.ActorID != "u9" || got.ActorLabel != "Rina" || got.ActorType != record.ActorUser {
		t.Fatalf("%+v", got)
	}
	// Actor eksplisit menang atas identitas ctx.
	rec.Record(userCtx("Budi"), Event{Action: record.ActionAPISpecUploaded, ProjectID: "p1", Actor: &base.Actor{ID: "u9", Label: "Rina"}})
	if got.ActorID != "u9" {
		t.Fatalf("%+v", got)
	}
	// Tanpa Actor: perilaku lama (dari ctx).
	rec.Record(userCtx("Budi"), Event{Action: record.ActionAPISpecUploaded, ProjectID: "p1"})
	if got.ActorID != "u1" || got.ActorLabel != "Budi" {
		t.Fatalf("%+v", got)
	}
}
