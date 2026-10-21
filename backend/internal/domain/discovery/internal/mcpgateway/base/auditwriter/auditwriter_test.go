package auditwriter

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

func logger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestWrite_Success(t *testing.T) {
	log, _ := logger()
	b := bus.NewRegistry(log)
	var got []record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		got = append(got, req.(record.Request))
		return record.Response{Recorded: true}, nil
	})
	req := record.Request{Action: record.ActionMCPInitialize}
	New(b, log).Write(context.Background(), req)
	if len(got) != 1 || got[0].Action != req.Action {
		t.Fatalf("%v", got)
	}
}

func TestWrite_ErrorFailOpenNoArgsInLog(t *testing.T) {
	log, buf := logger()
	b := bus.NewRegistry(log)
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) {
		return nil, errors.New("db down")
	})
	New(b, log).Write(context.Background(), record.Request{
		Action: record.ActionMCPReadFile,
		Args:   map[string]any{"path": "SECRET-ARG"}, Result: map[string]any{"x": "SECRET-RESULT"},
	})
	out := buf.String()
	if !strings.Contains(out, "audit write failed") || !strings.Contains(out, "mcp.read_file") {
		t.Fatal(out)
	}
	if strings.Contains(out, "SECRET") {
		t.Fatalf("Args/Result bocor ke log: %s", out)
	}
}

func TestWrite_NotRegistered(t *testing.T) {
	log, buf := logger()
	New(bus.NewRegistry(log), log).Write(context.Background(), record.Request{Action: record.ActionMCPToolsList})
	if !strings.Contains(buf.String(), "audit write failed") {
		t.Fatal(buf.String())
	}
}

func TestWrite_CancelledContextStillWrites(t *testing.T) {
	log, _ := logger()
	b := bus.NewRegistry(log)
	var ctxErr error
	called := false
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(ctx context.Context, _ any) (any, error) {
		called, ctxErr = true, ctx.Err()
		return record.Response{Recorded: true}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	New(b, log).Write(ctx, record.Request{Action: record.ActionMCPToolsList})
	if !called || ctxErr != nil {
		t.Fatalf("called=%v ctxErr=%v", called, ctxErr)
	}
}
