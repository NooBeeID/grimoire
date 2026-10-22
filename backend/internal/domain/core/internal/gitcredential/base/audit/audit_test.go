package audit

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
)

func TestRecord(t *testing.T) {
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var got record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, req any) (any, error) {
		got = req.(record.Request)
		return record.Response{Recorded: true}, nil
	})
	New(b, nil).Record(context.Background(), Event{
		Actor: base.Actor{ID: "u1", Label: "Rina"}, Action: record.ActionGitCredentialSaved,
		Provider: base.ProviderGitLab, Host: "git.noobee.id",
		Args: map[string]any{"replaced": true, "token": "ghp_CANARY"},
	})
	if got.Action != "git_credential.saved" || got.TargetType != "git_credential" || got.Target != "gitlab:git.noobee.id" ||
		got.ActorID != "u1" || got.Status != record.StatusOK {
		t.Fatalf("%+v", got)
	}
	if got.Args["replaced"] != true || got.Args["provider"] != "gitlab" {
		t.Fatalf("%v", got.Args)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "CANARY") {
		t.Fatal("args hanya boleh key whitelist")
	}
}

func TestRecordFailOpen(t *testing.T) {
	b := bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	New(b, nil).Record(context.Background(), Event{Action: record.ActionGitCredentialDeleted}) // handler tak ada: tidak panik
}
