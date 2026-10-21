package listendpoints

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/listendpoints"
)

const pid = "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"

type fakeReader struct {
	names              []string
	rows               []Row
	total              int
	namesErr, queryErr error
	nameCalls, qCalls  int
	gotFilter          Filter
}

func (f *fakeReader) ListSpecNames(context.Context, string) ([]string, error) {
	f.nameCalls++
	return f.names, f.namesErr
}

func (f *fakeReader) QueryEndpoints(_ context.Context, fl Filter) ([]Row, int, error) {
	f.qCalls++
	f.gotFilter = fl
	return f.rows, f.total, f.queryErr
}

func rows(n int) []Row {
	r := make([]Row, n)
	for i := range r {
		r[i] = Row{Spec: "s", Endpoint: base.Endpoint{Method: "GET", Path: "/p", Tags: []string{}}}
	}
	return r
}

func exec(t *testing.T, f *fakeReader, req listendpoints.Request) listendpoints.Response {
	t.Helper()
	if req.ProjectID == "" {
		req.ProjectID = pid
	}
	res, err := NewUsecase(f).Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestExecute_InvalidProjectID(t *testing.T) {
	f := &fakeReader{}
	_, err := NewUsecase(f).Execute(context.Background(), listendpoints.Request{ProjectID: "x"})
	var e *errs.Error
	if !errors.Is(err, errs.ErrInvalid) || !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("err=%v", err)
	}
	if f.nameCalls+f.qCalls != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestExecute_NoSpecFilterSkipsNameLookup(t *testing.T) {
	f := &fakeReader{}
	res := exec(t, f, listendpoints.Request{})
	if f.nameCalls != 0 || !res.SpecFound {
		t.Fatalf("calls=%d found=%v", f.nameCalls, res.SpecFound)
	}
}

func TestExecute_UnknownSpecReturnsNamesNoQuery(t *testing.T) {
	f := &fakeReader{names: []string{"auth", "payment"}}
	res := exec(t, f, listendpoints.Request{Spec: "paymnt"})
	if res.SpecFound || !reflect.DeepEqual(res.Specs, []string{"auth", "payment"}) || res.Items == nil || len(res.Items) != 0 || f.qCalls != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestExecute_UnknownSpecProjectWithoutSpecs(t *testing.T) {
	res := exec(t, &fakeReader{names: nil}, listendpoints.Request{Spec: "x"})
	if res.Specs == nil || len(res.Specs) != 0 {
		t.Fatalf("%#v", res.Specs)
	}
}

func TestExecute_SpecIsCaseSensitive(t *testing.T) {
	res := exec(t, &fakeReader{names: []string{"payment"}}, listendpoints.Request{Spec: "Payment"})
	if res.SpecFound {
		t.Fatal("found")
	}
}

func TestExecute_KnownSpecQueriesWithFilter(t *testing.T) {
	f := &fakeReader{names: []string{"payment"}}
	res := exec(t, f, listendpoints.Request{Spec: "payment"})
	if !res.SpecFound || f.qCalls != 1 || f.gotFilter.Spec != "payment" || f.gotFilter.ProjectID != pid {
		t.Fatalf("%+v %+v", res, f.gotFilter)
	}
}

func TestExecute_PassesNormalizedFilterToRepo(t *testing.T) {
	f := &fakeReader{}
	exec(t, f, listendpoints.Request{Limit: 9999, Offset: -3, Query: "50%"})
	if f.gotFilter.Limit != 300 || f.gotFilter.Offset != 0 || f.gotFilter.Pattern != `%50\%%` {
		t.Fatalf("%+v", f.gotFilter)
	}
}

func TestExecute_Pagination(t *testing.T) {
	cases := []struct {
		name       string
		total, off int
		n          int
		trunc      bool
		next       int
	}{
		{"truncated", 250, 100, 100, true, 200},
		{"last page", 250, 200, 50, false, 0},
		{"exact fit", 100, 0, 100, false, 0},
	}
	for _, c := range cases {
		res := exec(t, &fakeReader{rows: rows(c.n), total: c.total}, listendpoints.Request{Offset: c.off, Limit: 100})
		if res.Truncated != c.trunc || res.NextOffset != c.next || res.Total != c.total {
			t.Errorf("%s: %+v", c.name, res)
		}
	}
}

func TestExecute_EmptyResultNonNil(t *testing.T) {
	res := exec(t, &fakeReader{}, listendpoints.Request{})
	if res.Items == nil || len(res.Items) != 0 || res.Total != 0 || !res.SpecFound {
		t.Fatalf("%+v", res)
	}
}

func TestExecute_MapsRow(t *testing.T) {
	op := "getX"
	f := &fakeReader{total: 2, rows: []Row{
		{Spec: "a", Endpoint: base.Endpoint{Method: "GET", Path: "/x"}},
		{Spec: "b", Endpoint: base.Endpoint{Method: "POST", Path: "/y", OperationID: &op, Summary: "S", Tags: []string{"t"}, Deprecated: true}},
	}}
	res := exec(t, f, listendpoints.Request{})
	if res.Items[0].OperationID != "" || res.Items[0].Tags == nil || len(res.Items[0].Tags) != 0 {
		t.Fatalf("%+v", res.Items[0])
	}
	want := listendpoints.Endpoint{Spec: "b", Method: "POST", Path: "/y", OperationID: "getX", Summary: "S", Tags: []string{"t"}, Deprecated: true}
	if !reflect.DeepEqual(res.Items[1], want) {
		t.Fatalf("%+v", res.Items[1])
	}
}

func TestExecute_RepoErrorsWrapped(t *testing.T) {
	boom := errors.New("boom")
	for _, f := range []*fakeReader{{namesErr: boom}, {names: []string{"x"}, queryErr: boom}} {
		_, err := NewUsecase(f).Execute(context.Background(), listendpoints.Request{ProjectID: pid, Spec: "x"})
		var e *errs.Error
		if !errors.Is(err, boom) || errors.As(err, &e) {
			t.Fatalf("err=%v", err)
		}
	}
}
