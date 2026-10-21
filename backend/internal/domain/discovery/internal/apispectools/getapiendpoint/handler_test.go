package getapiendpoint

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

type fakeBus struct {
	calls int
	reqs  []getendpoint.Request
}

func newTool(t *testing.T, maxResp int, h func(getendpoint.Request) (any, error)) (toolkit.Tool, *fakeBus) {
	t.Helper()
	reg := bus.NewRegistry(nil)
	f := &fakeBus{}
	if h != nil {
		reg.Register(constants.BUS_PROJECT_APISPEC_GET_ENDPOINT, func(_ context.Context, req any) (any, error) {
			f.calls++
			r, err := bus.Expect[getendpoint.Request](req)
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

func found(r getendpoint.Request) (any, error) {
	return getendpoint.Response{Found: true, Spec: "petstore", Method: "GET", Path: "/pets/{petId}", EffectiveDepth: 4,
		Operation: []byte(`{"secret-unik-123":{"$circular":"#/c/Pet"}}`), Servers: []byte(`[]`), SecuritySchemes: []byte(`{}`)}, nil
}

func TestHandler_InvalidArgs_NoBusCall(t *testing.T) {
	tool, f := newTool(t, 102400, found)
	for _, a := range []string{`{"method":"FETCH","path":"/a"}`, `{"operation_id":"x","schema_depth":9}`,
		`{"project_id":"P2","method":"GET","path":"/a"}`, `{"spec":"x"}`} {
		res, err := call(tool, "P1", a)
		if err != nil || !res.IsError || res.TargetType != "api_endpoint" || res.Summary["error_code"] != "invalid_argument" || res.Summary["field"] == nil {
			t.Fatalf("%s: %+v %v", a, res, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("bus tidak boleh dipanggil")
	}
}

func TestHandler_ForwardsProjectAndDefaults(t *testing.T) {
	tool, f := newTool(t, 102400, found)
	res, err := call(tool, "P1", `{"method":"get","path":"/pets/{petId}"}`)
	if err != nil || res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
	r := f.reqs[0]
	if r.ProjectID != "P1" || r.MaxBytes != 100352 || r.SchemaDepth != 4 || r.Method != "GET" || r.Path != "/pets/{petId}" {
		t.Fatalf("%+v", r)
	}
	if !json.Valid([]byte(res.Text)) || res.Target != "petstore GET /pets/{petId}" || res.TargetType != "api_endpoint" {
		t.Fatalf("%+v", res)
	}
	if len(res.Summary) != 6 {
		t.Fatalf("summary %v", res.Summary)
	}
	for _, k := range []string{"found", "schema_depth", "requested_depth", "truncated", "circular_count", "collapsed_count"} {
		if _, ok := res.Summary[k]; !ok {
			t.Fatal(k)
		}
	}
	b, _ := json.Marshal(res.Summary)
	if strings.Contains(string(b), "secret-unik-123") {
		t.Fatal("summary bocor isi")
	}
}

func TestHandler_SmallConfig(t *testing.T) {
	tool, f := newTool(t, 1024, found)
	if _, err := call(tool, "P", `{"operation_id":"x"}`); err != nil {
		t.Fatal(err)
	}
	if f.reqs[0].MaxBytes != 768 {
		t.Fatal(f.reqs[0].MaxBytes)
	}
}

func TestHandler_EndpointNotFound(t *testing.T) {
	tool, _ := newTool(t, 102400, func(getendpoint.Request) (any, error) {
		return getendpoint.Response{Reason: getendpoint.ReasonEndpointNotFound, Spec: "petstore",
			Similar: []getendpoint.EndpointRef{{Method: "GET", Path: "/pets/{petId}", OperationID: "showPetById", Summary: "Info"}}}, nil
	})
	res, err := call(tool, "P", `{"spec":"petstore","method":"GET","path":"/pet/{petId}"}`)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "Similar:") || !strings.Contains(res.Text, "list_api_endpoints") {
		t.Fatalf("%+v %v", res, err)
	}
	if res.Summary["error_code"] != "endpoint_not_found" || res.Summary["similar_count"] != 1 {
		t.Fatal(res.Summary)
	}
}

func TestHandler_SpecRequired(t *testing.T) {
	tool, _ := newTool(t, 102400, func(getendpoint.Request) (any, error) {
		return getendpoint.Response{Reason: getendpoint.ReasonSpecRequired, Specs: []string{"billing", "petstore"}}, nil
	})
	res, err := call(tool, "P", `{"method":"GET","path":"/pets"}`)
	if err != nil || !res.IsError || !strings.Contains(res.Text, "billing, petstore") {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHandler_Errors(t *testing.T) {
	tool, _ := newTool(t, 102400, nil)
	res, err := call(tool, "P", `{"operation_id":"x"}`)
	if !errors.Is(err, bus.ErrNotRegistered) || res.Text != "" || !strings.HasPrefix(err.Error(), "get_api_endpoint:") {
		t.Fatalf("%+v %v", res, err)
	}
	sentinel := errors.New("x")
	for _, e := range []error{sentinel, context.DeadlineExceeded} {
		tool, _ := newTool(t, 102400, func(getendpoint.Request) (any, error) { return nil, e })
		if _, err := call(tool, "P", `{"operation_id":"x"}`); !errors.Is(err, e) {
			t.Fatal(err)
		}
	}
	tool, _ = newTool(t, 102400, func(getendpoint.Request) (any, error) { return getendpoint.Response{Reason: "WEIRD"}, nil })
	if _, err := call(tool, "P", `{"operation_id":"x"}`); err == nil {
		t.Fatal("reason aneh harus error")
	}
}
