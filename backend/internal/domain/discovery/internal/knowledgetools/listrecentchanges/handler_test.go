package listrecentchanges

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
	"grimoire/lib/common/errs"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listspecs"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

type busFake struct {
	fileReqs []listchanges.Request
	specReqs []listspecs.Request
}

func newTestTool(t *testing.T, files func(listchanges.Request) (any, error), specs func(listspecs.Request) (any, error)) (toolkit.Tool, *busFake) {
	t.Helper()
	reg := bus.NewRegistry(nil)
	f := &busFake{}
	if files != nil {
		reg.Register(constants.BUS_PROJECT_KNOWLEDGE_LIST_CHANGES, func(_ context.Context, req any) (any, error) {
			r, err := bus.Expect[listchanges.Request](req)
			if err != nil {
				return nil, err
			}
			f.fileReqs = append(f.fileReqs, r)
			return files(r)
		})
	}
	if specs != nil {
		reg.Register(constants.BUS_PROJECT_APISPEC_LIST_SPECS, func(_ context.Context, req any) (any, error) {
			r, err := bus.Expect[listspecs.Request](req)
			if err != nil {
				return nil, err
			}
			f.specReqs = append(f.specReqs, r)
			return specs(r)
		})
	}
	return newTool(app.Deps{Bus: reg}, func() time.Time { return now }), f
}

func call(tool toolkit.Tool, project, args string) (toolkit.Result, error) {
	return tool.Handler(context.Background(), toolkit.Call{ProjectID: project, Arguments: json.RawMessage(args)})
}

func okFiles(listchanges.Request) (any, error) {
	return listchanges.Response{Items: []listchanges.Item{{Path: "docs/a.md", SizeBytes: 5, CreatedAt: now, UpdatedAt: now}}, Total: 1}, nil
}

func okSpecs(listspecs.Request) (any, error) {
	return listspecs.Response{Items: []listspecs.Spec{{Name: "petstore", UpdatedAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour)}}}, nil
}

func TestHandler_All(t *testing.T) {
	tool, f := newTestTool(t, okFiles, okSpecs)
	res, err := call(tool, "proj-A", `{"since":"1h"}`)
	if err != nil || res.IsError {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.TargetType != "" || res.Target != "" || res.Summary["count"] != 2 || res.Summary["total"] != 2 {
		t.Fatalf("res=%+v", res)
	}
	if !strings.Contains(res.Text, "- [file] docs/a.md · dibuat") || !strings.Contains(res.Text, "- [spec] petstore · diperbarui") {
		t.Fatalf("text:\n%s", res.Text)
	}
	if f.fileReqs[0].ProjectID != "proj-A" || f.specReqs[0].ProjectID != "proj-A" {
		t.Fatalf("project: %+v %+v", f.fileReqs, f.specReqs)
	}
}

func TestHandler_InvalidArgs_BusNotCalled(t *testing.T) {
	tool, f := newTestTool(t, okFiles, okSpecs)
	for _, args := range []string{`{}`, `{"since":"1d","kind":"specs","path":"docs"}`, `{"since":"1d","project_id":"B"}`, `{"since":"200d"}`} {
		res, err := call(tool, "A", args)
		if err != nil || !res.IsError || res.Summary["error_code"] != "invalid_argument" {
			t.Fatalf("%s: res=%+v err=%v", args, res, err)
		}
	}
	if len(f.fileReqs)+len(f.specReqs) != 0 {
		t.Fatal("bus dipanggil")
	}
}

func TestHandler_ErrInvalid_BecomesIsError(t *testing.T) {
	tool, _ := newTestTool(t, func(listchanges.Request) (any, error) { return nil, fmt.Errorf("x: %w", errs.ErrInvalid) }, okSpecs)
	res, err := call(tool, "p", `{"since":"1d"}`)
	if err != nil || !res.IsError || res.Summary["error_code"] != "invalid_request" || !strings.HasSuffix(res.Text, "Periksa since dan path.") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestHandler_OtherError_IsGoError(t *testing.T) {
	want := errors.New("db down")
	tool, _ := newTestTool(t, func(listchanges.Request) (any, error) { return nil, want }, okSpecs)
	if _, err := call(tool, "p", `{"since":"1d"}`); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	empty, _ := newTestTool(t, nil, nil)
	if _, err := call(empty, "p", `{"since":"1d"}`); !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("err=%v", err)
	}
}

func TestInit_Tool(t *testing.T) {
	tool, _ := newTestTool(t, nil, nil)
	if tool.Name != "list_recent_changes" || tool.Title != "Perubahan terbaru" || tool.Handler == nil {
		t.Fatalf("tool=%+v", tool)
	}
	if !strings.HasPrefix(tool.Description, "Daftar file knowledge dan spec API yang berubah sejak waktu tertentu (read-only).") ||
		!strings.Contains(tool.Description, "file yang dihapus tidak muncul") {
		t.Fatalf("desc=%q", tool.Description)
	}
	var m map[string]any
	if err := json.Unmarshal(tool.InputSchema, &m); err != nil {
		t.Fatal(err)
	}
	props := m["properties"].(map[string]any)
	req := m["required"].([]any)
	if m["additionalProperties"] != false || len(req) != 1 || req[0] != "since" ||
		len(props["kind"].(map[string]any)["enum"].([]any)) != 3 || props["limit"].(map[string]any)["maximum"] != float64(50) {
		t.Fatalf("schema=%s", tool.InputSchema)
	}
	for k := range props {
		if strings.Contains(k, "project") {
			t.Fatalf("properti project: %s", k)
		}
	}
	if Init(app.Deps{Bus: bus.NewRegistry(nil)}).Name != "list_recent_changes" {
		t.Fatal("Init")
	}
}
