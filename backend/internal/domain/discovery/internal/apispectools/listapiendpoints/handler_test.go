package listapiendpoints

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/infra/app"
	"grimoire/internal/infra/bus"
	"grimoire/internal/infra/config"
	"grimoire/lib/constants"
	"grimoire/lib/contracts/discovery/mcpgateway/toolkit"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

type fakeBus struct {
	reg   *bus.Registry
	reqs  []listendpoints.Request
	resp  listendpoints.Response
	err   error
	calls int
}

func newFakeBus(register bool) *fakeBus {
	f := &fakeBus{reg: bus.NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	f.resp = listendpoints.Response{SpecFound: true, Total: 2, Items: []listendpoints.Endpoint{
		{Spec: "petstore", Method: "GET", Path: "/pets", OperationID: "listPets", Summary: "List all pets", Tags: []string{"pets"}},
		{Spec: "petstore", Method: "POST", Path: "/pets", OperationID: "createPet", Summary: "Create a pet", Tags: []string{"pets"}, Deprecated: true},
	}}
	if register {
		f.reg.Register(constants.BUS_PROJECT_APISPEC_LIST_ENDPOINTS, func(_ context.Context, req any) (any, error) {
			f.calls++
			if r, ok := req.(listendpoints.Request); ok {
				f.reqs = append(f.reqs, r)
			}
			return f.resp, f.err
		})
	}
	return f
}

func (f *fakeBus) call(projectID, args string) (toolkit.Result, error) {
	h := Init(app.Deps{Config: config.Config{MCPMaxResponseBytes: 102400}, Bus: f.reg}).Handler
	return h(context.Background(), toolkit.Call{ProjectID: projectID, Arguments: json.RawMessage(args)})
}

func TestHandler_InvalidArgs_BusNotCalled(t *testing.T) {
	f := newFakeBus(true)
	r, err := f.call("p", `{"limit":500}`)
	if err != nil || !r.IsError || f.calls != 0 || !strings.Contains(r.Text, "limit harus 1-300") {
		t.Fatalf("%+v %v %d", r, err, f.calls)
	}
	if r.Summary["error_code"] != "invalid_argument" || r.Summary["field"] != "limit" {
		t.Fatalf("%+v", r.Summary)
	}
}

func TestT09_AC6_ProjectIDIgnored(t *testing.T) {
	f := newFakeBus(true)
	r, err := f.call("P-A", `{"project_id":"P-B"}`)
	if err != nil || !r.IsError || f.calls != 0 || !strings.Contains(r.Text, "ditentukan oleh API key") {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := f.call("P-A", `{}`); err != nil || f.reqs[0].ProjectID != "P-A" {
		t.Fatalf("%v %+v", err, f.reqs)
	}
}

func TestHandler_Success_Text(t *testing.T) {
	f := newFakeBus(true)
	r, err := f.call("p", `{}`)
	if err != nil || r.IsError || r.TargetType != "api_spec" || f.reqs[0].Limit != 100 {
		t.Fatalf("%+v %v %+v", r, err, f.reqs)
	}
	for _, s := range []string{"Endpoint 1-2 dari 2 | spec: semua", "== petstore ==", "GET /pets — List all pets [listPets] (pets)"} {
		if !strings.Contains(r.Text, s) {
			t.Errorf("tidak memuat %q:\n%s", s, r.Text)
		}
	}
}

func TestHandler_SpecNotFound(t *testing.T) {
	f := newFakeBus(true)
	f.resp = listendpoints.Response{SpecFound: false, Specs: []string{"billing", "petstore"}}
	r, err := f.call("p", `{"spec":"petstor"}`)
	if err != nil || !r.IsError || r.Target != "petstor" ||
		!strings.Contains(r.Text, "Spec tidak ditemukan: petstor") || !strings.Contains(r.Text, "billing, petstore") {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHandler_Total0(t *testing.T) {
	f := newFakeBus(true)
	f.resp = listendpoints.Response{SpecFound: true}
	r, err := f.call("p", `{"tag":"x"}`)
	if err != nil || r.IsError || !strings.Contains(r.Text, "Tidak ada endpoint yang cocok") {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHandler_OffsetOutOfRange(t *testing.T) {
	f := newFakeBus(true)
	f.resp = listendpoints.Response{SpecFound: true, Total: 12}
	r, err := f.call("p", `{"offset":5000}`)
	if err != nil || !r.IsError || r.Summary["error_code"] != "offset_out_of_range" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestHandler_ProviderError_Wrapped(t *testing.T) {
	f := newFakeBus(true)
	sentinel := errors.New("boom")
	f.err = sentinel
	r, err := f.call("p", `{}`)
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "list_api_endpoints:") || r.Text != "" {
		t.Fatalf("%+v %v", r, err)
	}
	f.err = context.DeadlineExceeded
	if _, err := f.call("p", `{}`); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%v", err)
	}
}

func TestHandler_EndpointNotRegistered(t *testing.T) {
	f := newFakeBus(false)
	if _, err := f.call("p", `{}`); !errors.Is(err, bus.ErrNotRegistered) {
		t.Fatalf("%v", err)
	}
}

func TestHandler_Summary_NoSpecText(t *testing.T) {
	f := newFakeBus(true)
	r, _ := f.call("p", `{"query":"pets"}`)
	for k, v := range r.Summary {
		if s, ok := v.(string); ok {
			t.Errorf("summary %s memuat string %q", k, s)
		}
	}
}
