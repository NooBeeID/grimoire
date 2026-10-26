package listspecsbus

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/apispec/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/apispec/listspecs"
)

const pid = "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"

type fakeLister struct {
	items []base.SpecSummary
	err   error
	calls int
	got   string
}

func (f *fakeLister) ListSpecs(_ context.Context, projectID string) ([]base.SpecSummary, error) {
	f.calls++
	f.got = projectID
	return f.items, f.err
}

func TestExecute_MapsFields(t *testing.T) {
	ts := time.Date(2026, 10, 24, 17, 0, 0, 0, time.FixedZone("x", 7*3600))
	ts2 := time.Date(2026, 10, 20, 8, 0, 0, 0, time.FixedZone("y", -5*3600))
	sp := "p.yaml"
	f := &fakeLister{items: []base.SpecSummary{{Name: "payment", Title: "Payment API", APIVersion: "1.4.0", OpenAPIVersion: "3.0.3", EndpointCount: 12, UpdatedAt: ts, CreatedAt: ts2, SizeBytes: 321, SourcePath: &sp, CreatedByLabel: "x"}}}
	res, err := NewUsecase(f).Execute(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	want := listspecs.Spec{Name: "payment", Title: "Payment API", ApiVersion: "1.4.0", OpenapiVersion: "3.0.3", EndpointCount: 12, UpdatedAt: ts.UTC(), CreatedAt: ts2.UTC(), SizeBytes: 321}
	if len(res.Items) != 1 || res.Items[0] != want {
		t.Fatalf("got %+v", res.Items)
	}
	if res.Items[0].CreatedAt.Location() != time.UTC || res.Items[0].UpdatedAt.Location() != time.UTC {
		t.Fatal("not UTC")
	}
}

func TestExecute_PreservesStoreOrder(t *testing.T) {
	f := &fakeLister{items: []base.SpecSummary{{Name: "auth"}, {Name: "notification"}, {Name: "payment"}}}
	res, _ := NewUsecase(f).Execute(context.Background(), pid)
	for i, n := range []string{"auth", "notification", "payment"} {
		if res.Items[i].Name != n {
			t.Fatalf("order: %+v", res.Items)
		}
	}
}

func TestExecute_EmptyReturnsEmptySliceNotNil(t *testing.T) {
	for _, items := range [][]base.SpecSummary{nil, {}} {
		res, err := NewUsecase(&fakeLister{items: items}).Execute(context.Background(), pid)
		if err != nil || res.Items == nil || len(res.Items) != 0 {
			t.Fatalf("res=%#v err=%v", res, err)
		}
	}
}

func TestExecute_InvalidUUID(t *testing.T) {
	for _, id := range []string{"", "bukan-uuid", "123", "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e1", " " + pid} {
		f := &fakeLister{}
		_, err := NewUsecase(f).Execute(context.Background(), id)
		var e *errs.Error
		if !errors.Is(err, errs.ErrInvalid) || !errors.As(err, &e) || e.Code != errs.CodeInvalid {
			t.Fatalf("id=%q err=%v", id, err)
		}
		if f.calls != 0 {
			t.Fatalf("store dipanggil untuk %q", id)
		}
	}
}

func TestExecute_PassesProjectIDVerbatim(t *testing.T) {
	f := &fakeLister{}
	_, _ = NewUsecase(f).Execute(context.Background(), pid)
	if f.got != pid || f.calls != 1 {
		t.Fatalf("got=%q calls=%d", f.got, f.calls)
	}
}

func TestExecute_StoreErrorWrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewUsecase(&fakeLister{err: boom}).Execute(context.Background(), pid)
	if !errors.Is(err, boom) || errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
	if got := err.Error(); len(got) < 24 || got[:24] != "listspecsbus: list specs" {
		t.Fatalf("msg=%q", got)
	}
}

func TestExecute_DoesNotExposeContent(t *testing.T) {
	if n := reflect.TypeOf(listspecs.Spec{}).NumField(); n != 8 {
		t.Fatalf("fields=%d", n)
	}
}
