package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/knowledge/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/getproject"
)

func logger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func event() Event {
	return Event{
		Action: record.ActionKnowledgeFileCreated, ProjectID: "p1",
		Actor:      base.Actor{ID: "u1", Role: "developer", Label: "Budi"},
		TargetType: TargetFile, Target: "docs/a.md",
		Args: map[string]any{}, Result: map[string]any{"bytes": 12},
		Meta: base.RequestMeta{IP: "1.2.3.4", UserAgent: "ua"},
	}
}

func TestRecord_BuildsRequest(t *testing.T) {
	log, _ := logger()
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(_ context.Context, req any) (any, error) {
		if req.(getproject.Request).ProjectID != "p1" {
			t.Errorf("get-project req=%v", req)
		}
		return getproject.Response{Found: true, ID: "p1", Name: "Payments"}, nil
	})
	var got record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		got = req.(record.Request)
		return record.Response{Recorded: true}, nil
	})
	New(b, log).Record(reqctx.WithTraceID(context.Background(), "tr1"), event())
	if got.ActorType != record.ActorUser || got.ActorID != "u1" || got.ActorLabel != "Budi" ||
		got.ProjectID != "p1" || got.ProjectLabel != "Payments" || got.Action != record.ActionKnowledgeFileCreated ||
		got.TargetType != "file" || got.Target != "docs/a.md" || got.Status != record.StatusOK ||
		got.IP != "1.2.3.4" || got.UserAgent != "ua" || got.TraceID != "tr1" || got.Result["bytes"] != 12 {
		t.Fatalf("got %+v", got)
	}
}

func TestRecord_GetProjectFailureIsSoft(t *testing.T) {
	for name, h := range map[string]bus.Handler{
		"error":     func(context.Context, any) (any, error) { return nil, errors.New("down") },
		"not-found": func(context.Context, any) (any, error) { return getproject.Response{}, nil },
	} {
		t.Run(name, func(t *testing.T) {
			log, _ := logger()
			b := bus.NewRegistry(log)
			b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, h)
			var got record.Request
			calls := 0
			b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
				calls++
				got = req.(record.Request)
				return record.Response{}, nil
			})
			New(b, log).Record(context.Background(), event())
			if calls != 1 || got.ProjectLabel != "" {
				t.Fatalf("calls=%d label=%q", calls, got.ProjectLabel)
			}
		})
	}
}

func TestRecord_FailOpenAndNoContentInLog(t *testing.T) {
	log, buf := logger()
	b := bus.NewRegistry(log) // get-project dan record tidak terdaftar
	ev := event()
	ev.Args = map[string]any{"secret": "ISI-RAHASIA"}
	ev.Result = map[string]any{"secret": "ISI-RAHASIA"}
	New(b, log).Record(context.Background(), ev)
	out := buf.String()
	if !strings.Contains(out, "audit write failed") || !strings.Contains(out, "knowledge.file.created") {
		t.Fatalf("log: %s", out)
	}
	if strings.Contains(out, "ISI-RAHASIA") {
		t.Fatalf("log leaked args/result: %s", out)
	}
}

func TestNew_NilLogger(t *testing.T) {
	if New(bus.NewRegistry(nil), nil).log == nil {
		t.Fatal("log must default")
	}
}
