package listapiendpoints

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"grimoire/lib/contracts/project/apispec/listendpoints"
)

type fakeLister struct {
	reqs []listendpoints.Request
	resp listendpoints.Response
	err  error
}

func (f *fakeLister) List(_ context.Context, r listendpoints.Request) (listendpoints.Response, error) {
	f.reqs = append(f.reqs, r)
	return f.resp, f.err
}

func eps(n int) []listendpoints.Endpoint {
	out := make([]listendpoints.Endpoint, n)
	for i := range out {
		out[i] = listendpoints.Endpoint{Spec: "petstore", Method: "GET", Path: "/pets/" + string(rune('a'+i)), Summary: "Ringkasan panjang untuk baris"}
	}
	return out
}

func TestExecute_ForwardsRequest(t *testing.T) {
	f := &fakeLister{resp: listendpoints.Response{SpecFound: true}}
	_, err := newUsecase(f, 10000).Execute(context.Background(), "proj-1", Params{Spec: "petstore", Tag: "pets", Query: "pet", Limit: 50, Offset: 10})
	want := listendpoints.Request{ProjectID: "proj-1", Spec: "petstore", Tag: "pets", Query: "pet", Limit: 50, Offset: 10}
	if err != nil || len(f.reqs) != 1 || f.reqs[0] != want {
		t.Fatalf("%v %+v", err, f.reqs)
	}
}

func TestExecute_LimitAlwaysExplicit(t *testing.T) {
	p, _ := Parse(json.RawMessage(`{}`))
	f := &fakeLister{resp: listendpoints.Response{SpecFound: true}}
	if _, err := newUsecase(f, 10000).Execute(context.Background(), "p", p); err != nil || f.reqs[0].Limit != 100 {
		t.Fatalf("%v %+v", err, f.reqs)
	}
}

func TestExecute_BuildsPage(t *testing.T) {
	f := &fakeLister{resp: listendpoints.Response{SpecFound: true, Items: eps(3), Total: 3}}
	out, err := newUsecase(f, 100000).Execute(context.Background(), "p", Params{Limit: 100})
	if err != nil || out.Miss != nil || out.Page.Total != 3 || out.Result().IsError {
		t.Fatalf("%v %+v", err, out)
	}
	// budget kecil memotong baris
	f.resp = listendpoints.Response{SpecFound: true, Items: eps(20), Total: 20}
	out, _ = newUsecase(f, 400).Execute(context.Background(), "p", Params{Limit: 100})
	if _, shown := out.Page.Lines(); shown >= 20 || shown < 1 {
		t.Fatalf("shown=%d", shown)
	}
}

func TestExecute_SpecNotFound_ReturnsMiss(t *testing.T) {
	f := &fakeLister{resp: listendpoints.Response{SpecFound: false, Specs: []string{"a", "b"}}}
	out, err := newUsecase(f, 10000).Execute(context.Background(), "p", Params{Spec: "x", Limit: 100})
	if err != nil || out.Miss == nil || !out.Result().IsError {
		t.Fatalf("%v %+v", err, out)
	}
}

func TestExecute_PortError_Propagated(t *testing.T) {
	sentinel := errors.New("boom")
	f := &fakeLister{err: sentinel}
	out, err := newUsecase(f, 10000).Execute(context.Background(), "p", Params{Limit: 100})
	if !errors.Is(err, sentinel) || out.Miss != nil || out.Page.Total != 0 {
		t.Fatalf("%v %+v", err, out)
	}
}
