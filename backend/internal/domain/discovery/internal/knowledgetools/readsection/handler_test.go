package readsection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
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

func newTool(t *testing.T, maxResp int, h func(rf.Request) (any, error)) (toolkit.Tool, *busFake) {
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
	return Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: maxResp}, Bus: reg}), f
}

func call(tool toolkit.Tool, project, args string) (toolkit.Result, error) {
	return tool.Handler(context.Background(), toolkit.Call{ProjectID: project, Arguments: json.RawMessage(args)})
}

func serve(content string) func(rf.Request) (any, error) {
	return func(r rf.Request) (any, error) {
		return rf.Response{Found: true, Path: r.Path, Content: content, SizeBytes: len(content), TotalBytes: len(content)}, nil
	}
}

func TestHandler_Section(t *testing.T) {
	tool, _ := newTool(t, 102400, serve("# A\nx\n## B\ny\n# C\n"))
	res, err := call(tool, "p", `{"path":"a.md","heading":"B"}`)
	if err != nil || res.IsError || res.Text != "## B\ny\n" || res.Target != "a.md" || res.TargetType != "file" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_HeadingNotFound_IsError(t *testing.T) {
	tool, _ := newTool(t, 102400, serve("# A\n"))
	res, err := call(tool, "p", `{"path":"a.md","heading":"Nope"}`)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "get_file_outline") || res.Summary["error_code"] != "heading_not_found" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_SummaryHasNoContent(t *testing.T) {
	tool, _ := newTool(t, 102400, serve("# A\nSECRET-CONTENT-123\n"))
	res, err := call(tool, "p", `{"path":"a.md","heading":"A"}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res.Summary)
	if strings.Contains(string(b), "SECRET-CONTENT-123") {
		t.Fatalf("summary bocor: %s", b)
	}
	for k, v := range res.Summary {
		if _, ok := v.(string); ok {
			t.Fatalf("string di %s", k)
		}
	}
}

func TestHandler_ProjectFromCallOnly(t *testing.T) {
	tool, f := newTool(t, 102400, serve("# A\n"))
	res, _ := call(tool, "A", `{"path":"a.md","heading":"A","project_id":"B"}`)
	if !res.IsError || !strings.Contains(res.Text, "ditentukan oleh API key") || f.calls != 0 {
		t.Fatalf("res=%+v calls=%d", res, f.calls)
	}
	if _, err := call(tool, "A", `{"path":"a.md","heading":"A"}`); err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 1 || f.reqs[0].ProjectID != "A" {
		t.Fatalf("reqs=%+v", f.reqs)
	}
}

func TestHandler_Errors(t *testing.T) {
	tool, _ := newTool(t, 102400, func(rf.Request) (any, error) { return nil, fmt.Errorf("x: %w", errs.ErrInvalid) })
	res, err := call(tool, "p", `{"path":"a.md","heading":"A"}`)
	if err != nil || !res.IsError || res.Summary["error_code"] != "invalid_request" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	want := errors.New("db down")
	tool, _ = newTool(t, 102400, func(rf.Request) (any, error) { return nil, want })
	if _, err := call(tool, "p", `{"path":"a.md","heading":"A"}`); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}

func TestInit_Tool(t *testing.T) {
	tool, _ := newTool(t, 102400, nil)
	if tool.Name != "read_section" || tool.Title != "Baca satu bagian file knowledge" || tool.Handler == nil {
		t.Fatalf("tool=%+v", tool)
	}
	if !strings.HasPrefix(tool.Description, "Baca satu bagian file Markdown knowledge (read-only), dari heading sampai sebelum heading selevel atau lebih tinggi.") {
		t.Fatalf("desc=%q", tool.Description)
	}
	small, _ := newTool(t, 1024, nil)
	var m map[string]any
	if err := json.Unmarshal(small.InputSchema, &m); err != nil {
		t.Fatal(err)
	}
	props := m["properties"].(map[string]any)
	if props["limit"].(map[string]any)["maximum"] != float64(base.MaxReadLimit(1024)) || m["additionalProperties"] != false {
		t.Fatalf("schema=%s", small.InputSchema)
	}
	req := m["required"].([]any)
	if len(req) != 2 || req[0] != "path" || req[1] != "heading" {
		t.Fatalf("required=%v", req)
	}
	for k := range props {
		if strings.Contains(k, "project") {
			t.Fatalf("properti project: %s", k)
		}
	}
}
