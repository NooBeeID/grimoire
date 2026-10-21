package getapiendpoint

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
	"grimoire/lib/contracts/project/apispec/getendpoint"
)

type fakeRepo struct {
	reqs []getendpoint.Request
	resp getendpoint.Response
	err  error
}

func (f *fakeRepo) Get(_ context.Context, r getendpoint.Request) (getendpoint.Response, error) {
	f.reqs = append(f.reqs, r)
	return f.resp, f.err
}

func foundResp() getendpoint.Response {
	return getendpoint.Response{Found: true, Spec: "petstore", Method: "GET", Path: "/pets", EffectiveDepth: 4,
		Operation: []byte(`{"operationId":"x"}`)}
}

func TestExecute_ForwardsRequest(t *testing.T) {
	f := &fakeRepo{resp: foundResp()}
	uc := newUsecase(f, 5000)
	p := Params{Ask: base.EndpointAsk{Spec: "s", Method: "GET", Path: "/p", OperationID: ""}, SchemaDepth: 3}
	if _, err := uc.Execute(context.Background(), "P1", p); err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 1 {
		t.Fatal("harus satu panggilan")
	}
	want := getendpoint.Request{ProjectID: "P1", Spec: "s", Method: "GET", Path: "/p", SchemaDepth: 3, MaxBytes: 5000}
	if f.reqs[0] != want {
		t.Fatalf("%+v", f.reqs[0])
	}
}

func TestExecute_Shapes(t *testing.T) {
	cases := []struct {
		resp getendpoint.Response
		kind string
	}{
		{foundResp(), "detail"},
		{getendpoint.Response{Reason: getendpoint.ReasonSpecRequired, Specs: []string{"a", "b"}}, "specmiss"},
		{getendpoint.Response{Reason: getendpoint.ReasonSpecNotFound, Specs: []string{"a"}}, "specmiss"},
		{getendpoint.Response{Reason: getendpoint.ReasonEndpointNotFound, Spec: "a"}, "miss"},
	}
	for _, c := range cases {
		lk, err := newUsecase(&fakeRepo{resp: c.resp}, 100).Execute(context.Background(), "P", Params{SchemaDepth: 4})
		if err != nil {
			t.Fatal(err)
		}
		var got string
		switch lk.(type) {
		case base.EndpointDetail:
			got = "detail"
		case base.SpecMiss:
			got = "specmiss"
		case base.EndpointMiss:
			got = "miss"
		}
		if got != c.kind {
			t.Fatalf("%s != %s (%T)", got, c.kind, lk)
		}
	}
}

func TestExecute_Errors(t *testing.T) {
	sentinel := errors.New("boom")
	for _, e := range []error{sentinel, context.DeadlineExceeded} {
		_, err := newUsecase(&fakeRepo{err: e}, 100).Execute(context.Background(), "P", Params{})
		if !errors.Is(err, e) {
			t.Fatal(err)
		}
	}
	if _, err := newUsecase(&fakeRepo{resp: getendpoint.Response{Reason: "WEIRD"}}, 100).Execute(context.Background(), "P", Params{}); err == nil {
		t.Fatal("reason aneh harus error")
	}
}
