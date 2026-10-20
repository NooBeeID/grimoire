package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

func newRec(t *testing.T) (*BusRecorder, *bus.Registry, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	b := bus.NewRegistry(log)
	return New(b, log), b, &buf
}

func capture(b *bus.Registry) *[]record.Request {
	var got []record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(ctx context.Context, req any) (any, error) {
		r, err := bus.Expect[record.Request](req)
		if err != nil {
			return nil, err
		}
		got = append(got, r)
		return record.Response{Recorded: true}, nil
	})
	return &got
}

func TestRecord_SendsCorrectRequest(t *testing.T) {
	rec, b, _ := newRec(t)
	got := capture(b)
	ctx := reqctx.WithTraceID(reqctx.WithIdentity(context.Background(),
		reqctx.Identity{UserID: "u1", Email: "a@tim.id", Role: "super_admin"}), "trace-1")
	rec.Record(ctx, Event{
		Action: "user.created", TargetType: "user", Target: "u2",
		Args: map[string]any{"email": "b@tim.id"}, Meta: base.RequestMeta{IP: "10.0.0.5", UserAgent: "ua"},
	})
	if len(*got) != 1 {
		t.Fatalf("n=%d", len(*got))
	}
	r := (*got)[0]
	if r.ActorType != record.ActorUser || r.ActorID != "u1" || r.ActorLabel != "a@tim.id" || r.Action != "user.created" ||
		r.TargetType != "user" || r.Target != "u2" || r.Status != record.StatusOK || r.IP != "10.0.0.5" ||
		r.UserAgent != "ua" || r.TraceID != "trace-1" || r.Args["email"] != "b@tim.id" {
		t.Fatalf("%+v", r)
	}
}

func TestRecord_ActorResolution(t *testing.T) {
	rec, b, _ := newRec(t)
	got := capture(b)
	rec.Record(context.Background(), Event{Action: "auth.login.failed", Status: record.StatusDenied})
	rec.Record(context.Background(), Event{Action: "user.created", Actor: &Actor{Type: record.ActorSystem, Label: "cli"}})
	if (*got)[0].ActorType != record.ActorAnonymous || (*got)[0].ActorID != "" || (*got)[0].Status != record.StatusDenied {
		t.Fatalf("anonymous: %+v", (*got)[0])
	}
	if (*got)[1].ActorType != record.ActorSystem || (*got)[1].ActorLabel != "cli" {
		t.Fatalf("explicit: %+v", (*got)[1])
	}
}

func TestRecord_UnregisteredEndpointLogsAndDoesNotPanic(t *testing.T) {
	rec, _, buf := newRec(t)
	ctx := reqctx.WithTraceID(context.Background(), "trace-9")
	rec.Record(ctx, Event{Action: "auth.logout"})
	out := buf.String()
	if !strings.Contains(out, "audit record gagal") || !strings.Contains(out, "auth.logout") || !strings.Contains(out, "trace-9") {
		t.Fatalf("log: %s", out)
	}
}

func TestRecord_HandlerErrorIsFailOpen(t *testing.T) {
	rec, b, buf := newRec(t)
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(context.Context, any) (any, error) { return nil, errors.New("db down") })
	rec.Record(context.Background(), Event{Action: "x"})
	if !strings.Contains(buf.String(), "audit record gagal") {
		t.Fatal("error harus di-log")
	}
}

func TestRecord_StripsSensitiveKeys(t *testing.T) {
	rec, b, _ := newRec(t)
	got := capture(b)
	args := map[string]any{
		"email": "a@tim.id", "password": "p", "New_Password": "p", "session_token": "t", "api_secret": "s",
		"token_hash": "h", "nested": map[string]any{"password": "p", "keep": 1},
	}
	rec.Record(context.Background(), Event{Action: "x", Args: args, Result: map[string]any{"hash": "h", "ok": true}})
	r := (*got)[0]
	for _, k := range []string{"password", "New_Password", "session_token", "api_secret", "token_hash"} {
		if _, ok := r.Args[k]; ok {
			t.Errorf("kunci %s tidak dibuang", k)
		}
	}
	if r.Args["email"] != "a@tim.id" || r.Args["nested"].(map[string]any)["keep"] != 1 {
		t.Fatalf("%v", r.Args)
	}
	if _, ok := r.Args["nested"].(map[string]any)["password"]; ok {
		t.Fatal("kunci bersarang tidak dibuang")
	}
	if _, ok := r.Result["hash"]; ok || r.Result["ok"] != true {
		t.Fatalf("%v", r.Result)
	}
	if _, ok := args["password"]; !ok {
		t.Fatal("input asli tidak boleh dimutasi")
	}
}
