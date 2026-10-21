package getprojectbus

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/contracts/project/management/getproject"
)

type fakeFinder struct {
	p     base.Project
	found bool
	err   error
	gotID []string
}

func (f *fakeFinder) FindProject(_ context.Context, id string) (base.Project, bool, error) {
	f.gotID = append(f.gotID, id)
	return f.p, f.found, f.err
}

func TestUsecase_Execute_Found(t *testing.T) {
	f := &fakeFinder{p: base.Project{ID: "id-1", Name: "Payments", Description: "x"}, found: true}
	res, err := NewUsecase(f).Execute(context.Background(), "id-1")
	if err != nil || res != (getproject.Response{Found: true, ID: "id-1", Name: "Payments"}) {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestUsecase_Execute_NotFound(t *testing.T) {
	res, err := NewUsecase(&fakeFinder{}).Execute(context.Background(), "x")
	if err != nil || res != (getproject.Response{}) {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestUsecase_Execute_RepoError(t *testing.T) {
	boom := errors.New("db down")
	res, err := NewUsecase(&fakeFinder{err: boom, found: true}).Execute(context.Background(), "x")
	if !errors.Is(err, boom) || res != (getproject.Response{}) {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestUsecase_Execute_PassesIDVerbatim(t *testing.T) {
	f := &fakeFinder{}
	uc := NewUsecase(f)
	for _, id := range []string{" 0192f5a0-1111-7aaa-8bbb-000000000001 ", ""} {
		res, err := uc.Execute(context.Background(), id)
		if err != nil || res.Found {
			t.Fatalf("id %q: %+v %v", id, res, err)
		}
	}
	if len(f.gotID) != 2 || f.gotID[0] != " 0192f5a0-1111-7aaa-8bbb-000000000001 " || f.gotID[1] != "" {
		t.Fatalf("gotID=%q", f.gotID)
	}
}

func TestUsecase_Execute_ReturnsOnlyIDAndName(t *testing.T) {
	typ := reflect.TypeFor[getproject.Response]()
	if typ.NumField() != 3 {
		t.Fatalf("fields=%d", typ.NumField())
	}
	for i, want := range []string{"Found", "ID", "Name"} {
		if typ.Field(i).Name != want {
			t.Fatalf("field %d = %s", i, typ.Field(i).Name)
		}
	}
}
