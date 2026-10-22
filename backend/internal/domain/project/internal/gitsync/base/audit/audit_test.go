package audit

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/gitsync/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/core/audit/record"
	"grimoire/lib/contracts/project/management/getproject"
)

func newReg() *bus.Registry { return bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil))) }

func TestRecord(t *testing.T) {
	b := newReg()
	var got record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, r any) (any, error) {
		got = r.(record.Request)
		return record.Response{Recorded: true}, nil
	})
	b.Register(constants.BUS_PROJECT_MANAGEMENT_GET_PROJECT, func(context.Context, any) (any, error) {
		return getproject.Response{Found: true, Name: "Proyek A"}, nil
	})
	New(b, nil).Record(context.Background(), Event{
		Actor: base.Actor{ID: "u1", Label: "Rina"}, ProjectID: "p1", Kind: base.KindAPISpec, Provider: "github", Host: "github.com",
		RepoPath: "noobee/docs", Branch: "main", Folder: "api", Status: base.StatusPartial,
		Summary: base.Summary{Created: 1, Rejected: 1},
	})
	if got.Action != "git_sync.run" || got.TargetType != "git_sync" || got.Target != "api_spec" || got.ProjectLabel != "Proyek A" || got.Status != record.StatusOK {
		t.Fatalf("%+v", got)
	}
	if got.Result["wait_count"] != 0 || got.Result["wait_total_seconds"] != 0 {
		t.Fatalf("tanpa tunggu: %v", got.Result)
	}
	if got.Result["rejected"] != 1 || got.Result["status"] != "partial" || got.Args["repo_path"] != "noobee/docs" {
		t.Fatalf("%v %v", got.Args, got.Result)
	}
}

func TestRecordFailedAndFailOpen(t *testing.T) {
	b := newReg()
	var got record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, r any) (any, error) {
		got = r.(record.Request)
		return record.Response{}, nil
	})
	// get-project tidak terdaftar: fail-soft.
	New(b, nil).Record(context.Background(), Event{Kind: base.KindKnowledge, Status: base.StatusFailed, ErrorCode: "GIT_NOT_FOUND"})
	if got.Status != record.StatusError || got.Result["error_code"] != "GIT_NOT_FOUND" || got.ProjectLabel != "" {
		t.Fatalf("%+v", got)
	}
	New(newReg(), nil).Record(context.Background(), Event{}) // audit tidak terdaftar: tidak panik
	if strings.Contains(fmt.Sprintf("%v", got.Args), "http") {
		t.Fatal("tidak boleh ada URL")
	}
}

func TestRecordWaitInfo(t *testing.T) {
	b := newReg()
	var got record.Request
	b.Register(constants.BUS_CORE_AUDIT_RECORD, func(_ context.Context, r any) (any, error) {
		got = r.(record.Request)
		return record.Response{}, nil
	})
	New(b, nil).Record(context.Background(), Event{Kind: base.KindKnowledge, Status: base.StatusSuccess,
		WaitCount: 2, WaitTotal: 1500 * time.Millisecond})
	if got.Result["wait_count"] != 2 || got.Result["wait_total_seconds"] != 2 {
		t.Fatalf("%v", got.Result)
	}
}
