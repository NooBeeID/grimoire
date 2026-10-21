package readfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	rf "grimoire/lib/contracts/project/knowledge/readfile"
)

const invalidHint = "Gunakan offset dari penanda terpotong atau 0."

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

func TestHandler_Found_Chunk(t *testing.T) {
	tool, f := newTool(t, 102400, func(r rf.Request) (any, error) {
		return rf.Response{Found: true, Path: r.Path, Content: "hello\n", SizeBytes: 6, TotalBytes: 100, NextOffset: 6, Truncated: true}, nil
	})
	res, err := call(tool, "proj-A", `{"path":"docs/a.md"}`)
	if err != nil || res.IsError || res.Structured != nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !strings.HasSuffix(res.Text, "\n\n[terpotong: lanjutkan dengan offset=6 dari total 100 byte]") {
		t.Fatalf("text=%q", res.Text)
	}
	if res.TargetType != "file" || res.Target != "docs/a.md" {
		t.Fatalf("target=%s/%s", res.TargetType, res.Target)
	}
	if f.reqs[0].ProjectID != "proj-A" || f.reqs[0].Limit != 40000 {
		t.Fatalf("req=%+v", f.reqs[0])
	}
}

func TestHandler_NotFound(t *testing.T) {
	tool, _ := newTool(t, 102400, func(rf.Request) (any, error) {
		return rf.Response{Found: false, Similar: []string{"docs/readme.md"}}, nil
	})
	res, err := call(tool, "p", `{"path":"docs/redme.md"}`)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "File tidak ditemukan: docs/redme.md") ||
		!strings.Contains(res.Text, "- docs/readme.md") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_ErrInvalid_BecomesIsError(t *testing.T) {
	tool, _ := newTool(t, 102400, func(rf.Request) (any, error) {
		return nil, fmt.Errorf("offset 9 di luar ukuran file (3 byte): %w", errs.ErrInvalid)
	})
	res, err := call(tool, "p", `{"path":"a","offset":9}`)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "Permintaan ditolak:") ||
		!strings.HasSuffix(res.Text, invalidHint) || res.Summary["error_code"] != "invalid_request" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_OtherError_IsGoError(t *testing.T) {
	for _, want := range []error{errors.New("db down"), context.DeadlineExceeded} {
		tool, _ := newTool(t, 102400, func(rf.Request) (any, error) { return nil, want })
		res, err := call(tool, "p", `{"path":"a"}`)
		if !errors.Is(err, want) || res.Text != "" || res.IsError {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	}
	tool, _ := newTool(t, 102400, nil)
	_, err := call(tool, "p", `{"path":"a"}`)
	if !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("err=%v", err)
	}
}

func TestHandler_SummaryHasNoContent(t *testing.T) {
	tool, _ := newTool(t, 102400, func(r rf.Request) (any, error) {
		return rf.Response{Found: true, Path: r.Path, Content: "SECRET-CONTENT-123", SizeBytes: 18, TotalBytes: 18, NextOffset: 18}, nil
	})
	res, err := call(tool, "p", `{"path":"a"}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res.Summary)
	if strings.Contains(string(b), "SECRET-CONTENT-123") {
		t.Fatalf("summary bocor: %s", b)
	}
	for k, v := range res.Summary {
		if _, ok := v.(string); ok {
			t.Fatalf("nilai string tak terduga di %s", k)
		}
	}
}

func TestHandler_InvalidArgs_BusNotCalled(t *testing.T) {
	tool, f := newTool(t, 102400, func(rf.Request) (any, error) { return rf.Response{Found: true}, nil })
	for _, args := range []string{`{}`, `{"path":"docs/"}`, `{"path":"a","limit":0}`, `{"path":"a","project_id":"B"}`} {
		res, err := call(tool, "A", args)
		if err != nil || !res.IsError || res.Summary["error_code"] != "invalid_argument" {
			t.Fatalf("%s: res=%+v err=%v", args, res, err)
		}
	}
	if f.calls != 0 {
		t.Fatalf("bus dipanggil %d kali", f.calls)
	}
}

func TestHandler_ProjectFromCallOnly(t *testing.T) {
	tool, f := newTool(t, 102400, func(rf.Request) (any, error) { return rf.Response{Found: true, TotalBytes: 0}, nil })
	res, _ := call(tool, "A", `{"path":"a","project_id":"B"}`)
	if !res.IsError || !strings.Contains(res.Text, "ditentukan oleh API key") {
		t.Fatalf("res=%+v", res)
	}
	if _, err := call(tool, "A", `{"path":"a"}`); err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 1 || f.reqs[0].ProjectID != "A" {
		t.Fatalf("reqs=%+v", f.reqs)
	}
}

func TestInit_Tool(t *testing.T) {
	tool, _ := newTool(t, 102400, nil)
	if tool.Name != "read_file" || tool.Title != "Baca file knowledge" || tool.Handler == nil {
		t.Fatalf("tool=%+v", tool)
	}
	if !strings.HasPrefix(tool.Description, "Baca isi satu file knowledge (read-only).") ||
		!strings.HasSuffix(tool.Description, "offset dan limit dalam byte.") {
		t.Fatalf("desc=%q", tool.Description)
	}
	if !json.Valid(tool.InputSchema) {
		t.Fatal("schema invalid")
	}
	small, _ := newTool(t, 1024, nil)
	var m map[string]any
	if err := json.Unmarshal(small.InputSchema, &m); err != nil {
		t.Fatal(err)
	}
	if m["properties"].(map[string]any)["limit"].(map[string]any)["maximum"] != float64(1024) {
		t.Fatalf("schema=%s", small.InputSchema)
	}
}
