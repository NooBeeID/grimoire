package listapispecs

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
	"grimoire/lib/contracts/project/apispec/listspecs"
)

func items(n int) []listspecs.Spec {
	out := make([]listspecs.Spec, n)
	ts := time.Date(2026, 10, 20, 8, 15, 0, 0, time.UTC)
	for i := range out {
		out[i] = listspecs.Spec{Name: fmt.Sprintf("s%02d", i+1), Title: "Service", ApiVersion: "1.0.0", OpenapiVersion: "3.0.3", EndpointCount: 2, UpdatedAt: ts}
	}
	return out
}

type env struct {
	tool  toolkit.Tool
	calls int
	last  listspecs.Request
}

func setup(t *testing.T, maxBytes int, resp listspecs.Response, err error, register bool) *env {
	t.Helper()
	e := &env{}
	reg := bus.NewRegistry(nil)
	if register {
		reg.Register(constants.BUS_PROJECT_APISPEC_LIST_SPECS, func(_ context.Context, req any) (any, error) {
			e.calls++
			e.last = req.(listspecs.Request)
			if err != nil {
				return nil, err
			}
			return resp, nil
		})
	}
	e.tool = Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: maxBytes}, Bus: reg})
	return e
}

func (e *env) call(args string) (toolkit.Result, error) {
	var raw json.RawMessage
	if args != "" {
		raw = json.RawMessage(args)
	}
	return e.tool.Handler(context.Background(), toolkit.Call{ProjectID: pid, Arguments: raw})
}

func TestHandler_Success(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{Items: items(3)}, nil, true)
	r, err := e.call("{}")
	if err != nil || r.IsError {
		t.Fatal(err, r.Text)
	}
	for _, s := range []string{"3 API spec di project ini:", "s01", "s02", "s03"} {
		if !strings.Contains(r.Text, s) {
			t.Errorf("tidak memuat %q", s)
		}
	}
	if e.last.ProjectID != pid || e.calls != 1 || r.Summary["count"] != 3 {
		t.Fatalf("%+v %d %v", e.last, e.calls, r.Summary)
	}
}

func TestHandler_NoArgsRawNil(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{Items: items(1)}, nil, true)
	for _, a := range []string{"", "{}"} {
		if r, err := e.call(a); err != nil || r.IsError {
			t.Fatal(a, err, r.Text)
		}
	}
	if e.calls != 2 {
		t.Fatal(e.calls)
	}
}

func TestHandler_ArgRejected_BusNotCalled(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{}, nil, true)
	r, err := e.call(`{"foo":1}`)
	if err != nil || !r.IsError || e.calls != 0 || r.TargetType != "api_spec" {
		t.Fatal(err, r, e.calls)
	}
}

func TestHandler_ProjectIDArg(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{Items: items(1)}, nil, true)
	r, err := e.call(`{"project_id":"22222222-2222-4222-8222-222222222222"}`)
	if err != nil || !r.IsError || !strings.Contains(r.Text, "ditentukan oleh API key") || e.calls != 0 {
		t.Fatal(err, r.Text, e.calls)
	}
	if _, err := e.call("{}"); err != nil || e.last.ProjectID != pid {
		t.Fatal(err, e.last)
	}
}

func TestHandler_BusNotRegistered(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{}, nil, false)
	r, err := e.call("{}")
	if err == nil || !errors.Is(err, bus.ErrNotRegistered) || !strings.HasPrefix(err.Error(), "list_api_specs: ") {
		t.Fatal(err)
	}
	if r.Text != "" || r.IsError {
		t.Fatalf("%+v", r)
	}
}

func TestHandler_ProviderError(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{}, errors.New("boom"), true)
	r, err := e.call("{}")
	if err == nil || !strings.Contains(err.Error(), "boom") || r.IsError {
		t.Fatal(err, r)
	}
}

func TestHandler_ProviderInvalidIsGoError(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{}, fmt.Errorf("x: %w", errs.ErrInvalid), true)
	r, err := e.call("{}")
	if err == nil || r.IsError {
		t.Fatal(err, r)
	}
}

func TestHandler_DeadlineExceeded(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{}, context.DeadlineExceeded, true)
	_, err := e.call("{}")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestHandler_EmptyProject(t *testing.T) {
	e := setup(t, 102400, listspecs.Response{Items: []listspecs.Spec{}}, nil, true)
	r, err := e.call("{}")
	if err != nil || r.IsError || !strings.HasPrefix(r.Text, "Belum ada API spec di project ini.") || r.Summary["count"] != 0 {
		t.Fatal(err, r)
	}
}

func TestHandler_BudgetFromConfig(t *testing.T) {
	e := setup(t, 1024, listspecs.Response{Items: items(30)}, nil, true)
	r, err := e.call("{}")
	if err != nil || r.IsError {
		t.Fatal(err)
	}
	if len(r.Text) > 512 || !strings.Contains(r.Text, "spec lain tidak ditampilkan") {
		t.Fatalf("len=%d\n%s", len(r.Text), r.Text)
	}
}

func TestHandler_SummaryNoSpecText(t *testing.T) {
	sp := items(2)
	sp[0].Name, sp[0].Title = "uniqname", "uniqtitle"
	e := setup(t, 102400, listspecs.Response{Items: sp}, nil, true)
	r, _ := e.call("{}")
	for k, v := range r.Summary {
		if _, ok := v.(int); !ok {
			t.Errorf("%s bukan int: %v", k, v)
		}
		if strings.Contains(fmt.Sprint(v), "uniq") {
			t.Errorf("bocor %s", k)
		}
	}
}

func TestInit_ToolMetadata(t *testing.T) {
	tool := setup(t, 102400, listspecs.Response{}, nil, true).tool
	if tool.Name != "list_api_specs" || tool.Title == "" || tool.Description == "" || tool.Handler == nil || !json.Valid(tool.InputSchema) {
		t.Fatalf("%+v", tool)
	}
}
