package getfileoutline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

type busFake struct {
	calls int
	reqs  []rf.Request
}

func newTool(t *testing.T, h func(rf.Request) (any, error)) (toolkit.Tool, *busFake) {
	t.Helper()
	reg := bus.NewRegistry(nil)
	f := &busFake{}
	if h != nil {
		reg.Register(constants.BUS_PROJECT_KNOWLEDGE_READ_FILE, func(_ context.Context, req any) (any, error) {
			f.calls++
			r, err := bus.Expect[rf.Request](req)
			if err != nil {
				return nil, err
			}
			f.reqs = append(f.reqs, r)
			return h(r)
		})
	}
	return Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: 102400}, Bus: reg}), f
}

func call(tool toolkit.Tool, project, args string) (toolkit.Result, error) {
	return tool.Handler(context.Background(), toolkit.Call{ProjectID: project, Arguments: json.RawMessage(args)})
}

func TestHandler_Outline(t *testing.T) {
	tool, f := newTool(t, func(r rf.Request) (any, error) {
		c := "# A\nx\n## B\n"
		return rf.Response{Found: true, Path: r.Path, Content: c, SizeBytes: len(c), TotalBytes: len(c), UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}, nil
	})
	res, err := call(tool, "proj-A", `{"path":"docs/a.md"}`)
	if err != nil || res.IsError {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.TargetType != "file" || res.Target != "docs/a.md" || res.Structured["updated_at"] != "2026-10-01T00:00:00Z" {
		t.Fatalf("res=%+v", res)
	}
	if f.reqs[0].ProjectID != "proj-A" || !f.reqs[0].Whole {
		t.Fatalf("req=%+v", f.reqs[0])
	}
}

func TestHandler_InvalidArgs_BusNotCalled(t *testing.T) {
	tool, f := newTool(t, func(rf.Request) (any, error) { return rf.Response{Found: true}, nil })
	for _, args := range []string{`{}`, `{"path":"docs/"}`, `{"path":"a.md","project_id":"B"}`} {
		res, err := call(tool, "A", args)
		if err != nil || !res.IsError || res.Summary["error_code"] != "invalid_argument" {
			t.Fatalf("%s: res=%+v err=%v", args, res, err)
		}
	}
	if f.calls != 0 {
		t.Fatalf("bus dipanggil %d kali", f.calls)
	}
}

func TestHandler_NotFound(t *testing.T) {
	tool, _ := newTool(t, func(rf.Request) (any, error) { return rf.Response{Found: false, Similar: []string{"docs/a.md"}}, nil })
	res, err := call(tool, "p", `{"path":"docs/b.md"}`)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "File tidak ditemukan: docs/b.md") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_ErrInvalid_BecomesIsError(t *testing.T) {
	tool, _ := newTool(t, func(rf.Request) (any, error) { return nil, fmt.Errorf("x: %w", errs.ErrInvalid) })
	res, err := call(tool, "p", `{"path":"a.md"}`)
	if err != nil || !res.IsError || res.Summary["error_code"] != "invalid_request" || !strings.HasSuffix(res.Text, "Periksa path dengan list_files.") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_OtherError_IsGoError(t *testing.T) {
	want := errors.New("db down")
	tool, _ := newTool(t, func(rf.Request) (any, error) { return nil, want })
	if _, err := call(tool, "p", `{"path":"a.md"}`); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	empty, _ := newTool(t, nil)
	if _, err := call(empty, "p", `{"path":"a.md"}`); !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("err=%v", err)
	}
}

func TestInit_Tool(t *testing.T) {
	tool, _ := newTool(t, nil)
	if tool.Name != "get_file_outline" || tool.Title != "Kerangka file knowledge" || tool.Handler == nil {
		t.Fatalf("tool=%+v", tool)
	}
	if !strings.HasPrefix(tool.Description, "Daftar heading satu file Markdown knowledge (read-only).") || !strings.Contains(tool.Description, "read_section") {
		t.Fatalf("desc=%q", tool.Description)
	}
	var m map[string]any
	if err := json.Unmarshal(tool.InputSchema, &m); err != nil {
		t.Fatal(err)
	}
	if m["additionalProperties"] != false || m["type"] != "object" {
		t.Fatalf("schema=%v", m)
	}
	req := m["required"].([]any)
	if len(req) != 1 || req[0] != "path" {
		t.Fatalf("required=%v", req)
	}
	for k := range m["properties"].(map[string]any) {
		if strings.Contains(k, "project") {
			t.Fatalf("properti project: %s", k)
		}
	}
}
