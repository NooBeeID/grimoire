package getendpoint

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

const (
	pidA = "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"
	pidB = "1b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e11"
)

const specYAML = `openapi: 3.0.3
info: {title: T, version: "1"}
paths:
  /orders:
    post:
      operationId: createOrder
      summary: Buat
      requestBody:
        content:
          application/json:
            schema: {$ref: '#/components/schemas/Order'}
      responses: {"201": {description: ok}}
  /pets/{petId}:
    get:
      operationId: getPet
      responses: {"200": {description: ok}}
  /pets:
    get:
      operationId: listPets
      responses: {"200": {description: ok}}
components:
  schemas:
    Order:
      type: object
      properties:
        customer: {$ref: '#/components/schemas/Customer'}
    Customer:
      type: object
      properties:
        address: {$ref: '#/components/schemas/Address'}
    Address:
      type: object
      properties: {street: {type: string}}
    Unused: {type: string}
`

type fakeRepo struct {
	specs                 map[string]map[string]base.Spec // project -> name -> spec
	eps                   []base.Endpoint
	namesCalls, findCalls int
	epCalls               int
	err                   error
	hideOnFind            bool
	gotProjects           []string
}

func (f *fakeRepo) ListSpecNames(_ context.Context, pid string) ([]string, error) {
	f.namesCalls++
	f.gotProjects = append(f.gotProjects, pid)
	if f.err != nil {
		return nil, f.err
	}
	var out []string
	for _, n := range []string{"a", "b", "c", "nope"} {
		if _, ok := f.specs[pid][n]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakeRepo) FindSpec(_ context.Context, pid, name string) (base.Spec, bool, error) {
	f.findCalls++
	f.gotProjects = append(f.gotProjects, pid)
	if f.err != nil {
		return base.Spec{}, false, f.err
	}
	if f.hideOnFind {
		return base.Spec{}, false, nil
	}
	s, ok := f.specs[pid][name]
	return s, ok, nil
}

func (f *fakeRepo) ListSpecEndpoints(context.Context, string) ([]base.Endpoint, error) {
	f.epCalls++
	return f.eps, f.err
}

func spec(name, content string) base.Spec {
	return base.Spec{ID: "id-" + name, Name: name, Content: content, Format: base.FormatYAML}
}

func newUC(f *fakeRepo) *Usecase {
	return NewUsecase(f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func one() *fakeRepo {
	sid := "createOrder"
	return &fakeRepo{
		specs: map[string]map[string]base.Spec{pidA: {"a": spec("a", specYAML)}},
		eps: []base.Endpoint{
			{Method: "POST", Path: "/orders", OperationID: &sid},
			{Method: "GET", Path: "/pets/{petId}"},
			{Method: "GET", Path: "/pets"},
		},
	}
}

func TestExecute_InvalidRequests(t *testing.T) {
	cases := map[string]getendpoint.Request{
		"pid x":         {ProjectID: "x", Method: "GET", Path: "/x"},
		"pid empty":     {Method: "GET", Path: "/x"},
		"all empty":     {ProjectID: pidA},
		"both modes":    {ProjectID: pidA, Method: "GET", Path: "/x", OperationID: "op"},
		"only method":   {ProjectID: pidA, Method: "GET"},
		"only path":     {ProjectID: pidA, Path: "/x"},
		"bad method":    {ProjectID: pidA, Method: "FETCH", Path: "/x"},
		"path no slash": {ProjectID: pidA, Method: "GET", Path: "x"},
		"opid+method":   {ProjectID: pidA, Method: "GET", OperationID: "op"},
	}
	for name, req := range cases {
		f := one()
		_, err := newUC(f).Execute(context.Background(), req)
		if !errors.Is(err, errs.ErrInvalid) {
			t.Fatalf("%s: err=%v", name, err)
		}
		if f.namesCalls+f.findCalls+f.epCalls != 0 {
			t.Fatalf("%s: repo dipanggil", name)
		}
	}
}

func TestNormalize_ClampAndTrim(t *testing.T) {
	for _, c := range []struct{ in, want int }{{0, 4}, {-1, 4}, {100, 8}, {3, 3}, {8, 8}} {
		p, err := normalize(getendpoint.Request{ProjectID: pidA, Method: " get ", Path: " /x ", SchemaDepth: c.in})
		if err != nil || p.depth != c.want || p.method != "GET" || p.path != "/x" {
			t.Fatalf("%+v err=%v", p, err)
		}
	}
	for _, c := range []struct{ in, want int }{{0, 102400}, {-5, 102400}, {500, 500}} {
		p, _ := normalize(getendpoint.Request{ProjectID: pidA, OperationID: "op", MaxBytes: c.in})
		if p.maxBytes != c.want {
			t.Fatalf("maxBytes %d -> %d", c.in, p.maxBytes)
		}
	}
}

func TestExecute_SpecSelection(t *testing.T) {
	ctx := context.Background()
	req := getendpoint.Request{ProjectID: pidA, OperationID: "getPet"}

	res, err := newUC(one()).Execute(ctx, req)
	if err != nil || !res.Found || res.Spec != "a" {
		t.Fatalf("single: %+v %v", res, err)
	}

	res, _ = newUC(&fakeRepo{}).Execute(ctx, req)
	if res.Found || res.Reason != getendpoint.ReasonSpecRequired || res.Specs == nil || len(res.Specs) != 0 || res.Similar == nil {
		t.Fatalf("zero: %+v", res)
	}

	f := &fakeRepo{specs: map[string]map[string]base.Spec{pidA: {"a": spec("a", specYAML), "b": spec("b", specYAML)}}}
	res, _ = newUC(f).Execute(ctx, req)
	if res.Reason != getendpoint.ReasonSpecRequired || strings.Join(res.Specs, ",") != "a,b" {
		t.Fatalf("two: %+v", res)
	}

	req.Spec = "zzz"
	res, _ = newUC(f).Execute(ctx, req)
	if res.Reason != getendpoint.ReasonSpecNotFound || strings.Join(res.Specs, ",") != "a,b" || res.Spec != "" {
		t.Fatalf("notfound: %+v", res)
	}
}

func TestExecute_NamedSpecSingleQuery(t *testing.T) {
	f := one()
	res, err := newUC(f).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, Spec: "a", OperationID: "getPet"})
	if err != nil || !res.Found || f.namesCalls != 0 || f.findCalls != 1 {
		t.Fatalf("res=%+v err=%v names=%d find=%d", res, err, f.namesCalls, f.findCalls)
	}
}

func TestExecute_RaceSpecDeleted(t *testing.T) {
	f := one()
	f.hideOnFind = true
	res, err := newUC(f).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, OperationID: "getPet"})
	if err != nil || res.Found || res.Reason != getendpoint.ReasonSpecNotFound || len(res.Specs) != 0 || res.Specs == nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestT06_AC7_ProjectIsolation(t *testing.T) {
	f := one()
	ctx := context.Background()
	res, _ := newUC(f).Execute(ctx, getendpoint.Request{ProjectID: pidB, Spec: "a", OperationID: "getPet"})
	if res.Reason != getendpoint.ReasonSpecNotFound || len(res.Specs) != 0 || res.Operation != nil {
		t.Fatalf("%+v", res)
	}
	res, _ = newUC(f).Execute(ctx, getendpoint.Request{ProjectID: pidB, OperationID: "getPet"})
	if res.Reason != getendpoint.ReasonSpecRequired || len(res.Specs) != 0 || res.Found {
		t.Fatalf("%+v", res)
	}
	for _, p := range f.gotProjects {
		if p != pidB {
			t.Fatalf("project bocor: %s", p)
		}
	}
}

func TestExecute_Lookup(t *testing.T) {
	ctx := context.Background()
	res, err := newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, Method: "post", Path: "/orders"})
	if err != nil || !res.Found || res.Method != "POST" || res.Path != "/orders" {
		t.Fatalf("%+v %v", res, err)
	}
	byID, _ := newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, OperationID: "createOrder"})
	if !byID.Found || byID.Method != "POST" || byID.Path != "/orders" || string(byID.Operation) != string(res.Operation) {
		t.Fatalf("%+v", byID)
	}
}

func TestT06_AC5_InlineDepth(t *testing.T) {
	ctx := context.Background()
	res, _ := newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, OperationID: "createOrder", SchemaDepth: 4})
	s := string(res.Operation)
	if strings.Contains(s, "$ref") || !strings.Contains(s, "street") || strings.Contains(s, "Unused") {
		t.Fatalf("depth4: %s", s)
	}
	res, _ = newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, OperationID: "createOrder", SchemaDepth: 1})
	if !strings.Contains(string(res.Operation), "#/components/schemas/Customer") || res.EffectiveDepth != 1 {
		t.Fatalf("depth1: %s", res.Operation)
	}
}

func TestExecute_EndpointNotFoundSimilar(t *testing.T) {
	f := one()
	res, err := newUC(f).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, Method: "GET", Path: "/pet/{id}"})
	if err != nil || res.Found || res.Reason != getendpoint.ReasonEndpointNotFound || res.Spec != "a" || res.Operation != nil {
		t.Fatalf("%+v %v", res, err)
	}
	if len(res.Similar) == 0 || len(res.Similar) > 5 || res.Similar[0].Path != "/pets/{petId}" || res.Method != "" || res.Path != "" {
		t.Fatalf("similar: %+v", res.Similar)
	}
	res, _ = newUC(f).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, OperationID: "nope"})
	if res.Reason != getendpoint.ReasonEndpointNotFound || res.Similar == nil {
		t.Fatalf("%+v", res)
	}
}

func TestExecute_SimilarMax5(t *testing.T) {
	f := one()
	for i := 0; i < 10; i++ {
		f.eps = append(f.eps, base.Endpoint{Method: "GET", Path: "/x" + string(rune('a'+i))})
	}
	res, _ := newUC(f).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, Method: "GET", Path: "/zzz"})
	if len(res.Similar) != 5 {
		t.Fatalf("len=%d", len(res.Similar))
	}
}

func TestExecute_Truncation(t *testing.T) {
	ctx := context.Background()
	res, _ := newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, OperationID: "createOrder"})
	if res.Truncated || res.Hint != "" {
		t.Fatalf("%+v", res)
	}
	res, _ = newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, OperationID: "createOrder", SchemaDepth: 8, MaxBytes: 230})
	if !res.Truncated || !strings.Contains(res.Hint, "turunkan schema_depth") || res.EffectiveDepth >= 8 {
		t.Fatalf("%+v", res)
	}
	res, _ = newUC(one()).Execute(ctx, getendpoint.Request{ProjectID: pidA, OperationID: "createOrder", MaxBytes: 5})
	var m map[string]any
	if err := json.Unmarshal(res.Operation, &m); err != nil || m["$truncated"] != true || m["operationId"] != "createOrder" {
		t.Fatalf("%s", res.Operation)
	}
}

func TestExecute_ServersAndSchemesNeverNil(t *testing.T) {
	res, _ := newUC(one()).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, OperationID: "getPet"})
	if string(res.Servers) != "[]" || string(res.SecuritySchemes) != "{}" {
		t.Fatalf("%s %s", res.Servers, res.SecuritySchemes)
	}
}

func TestExecute_CorruptStoredSpec(t *testing.T) {
	f := &fakeRepo{specs: map[string]map[string]base.Spec{pidA: {"a": spec("a", "{{{ rahasia-isi")}}}
	_, err := newUC(f).Execute(context.Background(), getendpoint.Request{ProjectID: pidA, OperationID: "x"})
	if err == nil || errors.Is(err, errs.ErrInvalid) || strings.Contains(err.Error(), "rahasia") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_RepoError(t *testing.T) {
	boom := errors.New("boom")
	for _, req := range []getendpoint.Request{
		{ProjectID: pidA, OperationID: "x"},
		{ProjectID: pidA, Spec: "a", OperationID: "x"},
	} {
		_, err := newUC(&fakeRepo{err: boom}).Execute(context.Background(), req)
		if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "getendpoint:") {
			t.Fatalf("err=%v", err)
		}
	}
}
