package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/apikey/base"
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
		Actor: base.Actor{ID: "u1", Role: "developer", Label: "u@x"}, Action: record.ActionAPIKeyCreated,
		ProjectID: "p1", KeyID: "k1", Args: map[string]any{"name": "ci-secret-name", "prefix": "abcdefgh"},
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
	ctx := reqctx.WithTraceID(context.Background(), "tr1")
	New(b, log).Record(ctx, event())
	want := record.Request{
		ActorType: record.ActorUser, ActorID: "u1", ActorLabel: "u@x", ProjectID: "p1", ProjectLabel: "Payments",
		Action: record.ActionAPIKeyCreated, TargetType: "api_key", Target: "k1",
		Args:   map[string]any{"name": "ci-secret-name", "prefix": "abcdefgh"},
		Status: record.StatusOK, IP: "1.2.3.4", UserAgent: "ua", TraceID: "tr1",
	}
	if got.ActorType != want.ActorType || got.ActorID != want.ActorID || got.ActorLabel != want.ActorLabel ||
		got.ProjectID != want.ProjectID || got.ProjectLabel != want.ProjectLabel || got.Action != want.Action ||
		got.TargetType != want.TargetType || got.Target != want.Target || got.Status != want.Status ||
		got.IP != want.IP || got.UserAgent != want.UserAgent || got.TraceID != want.TraceID ||
		got.Args["name"] != "ci-secret-name" || got.Args["prefix"] != "abcdefgh" {
		t.Fatalf("got %+v", got)
	}
}

func TestRecord_GetProjectFailSoft(t *testing.T) {
	cases := map[string]func(*bus.Registry){
		"not-found": func(b *bus.Registry) {
			b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(context.Context, any) (any, error) {
				return getproject.Response{}, nil
			})
		},
		"error": func(b *bus.Registry) {
			b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(context.Context, any) (any, error) {
				return nil, errors.New("down")
			})
		},
		"unregistered": func(*bus.Registry) {},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			log, _ := logger()
			b := bus.NewRegistry(log)
			setup(b)
			var got record.Request
			sent := false
			b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
				got, sent = req.(record.Request), true
				return record.Response{}, nil
			})
			New(b, log).Record(context.Background(), event())
			if !sent || got.ProjectLabel != "" {
				t.Fatalf("sent=%v label=%q", sent, got.ProjectLabel)
			}
		})
	}
}

func TestRecord_FailOpenAndNoArgsInLog(t *testing.T) {
	for name, setup := range map[string]func(*bus.Registry){
		"handler-error": func(b *bus.Registry) {
			b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) {
				return nil, errors.New("db down")
			})
		},
		"unregistered": func(*bus.Registry) {},
	} {
		t.Run(name, func(t *testing.T) {
			log, buf := logger()
			b := bus.NewRegistry(log)
			setup(b)
			New(b, log).Record(context.Background(), event()) // tidak boleh panic
			out := buf.String()
			if !strings.Contains(out, "audit record gagal") || !strings.Contains(out, "level=ERROR") {
				t.Fatalf("log=%s", out)
			}
			if strings.Contains(out, "ci-secret-name") {
				t.Fatalf("Args bocor di log: %s", out)
			}
		})
	}
}
