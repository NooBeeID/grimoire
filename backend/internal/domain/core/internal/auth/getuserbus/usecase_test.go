package getuserbus

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/contracts/core/auth/getuser"
)

type fakeRepo struct {
	user  base.User
	found bool
	err   error
	got   string
}

func (f *fakeRepo) FindUserByID(_ context.Context, id string) (base.User, bool, error) {
	f.got = id
	return f.user, f.found, f.err
}

const uid = "7d1e3b52-90aa-4f1c-8b2d-1c3e5a7b9d02"

func TestUsecase_Execute_Found(t *testing.T) {
	u := base.User{ID: uid, Name: "Sari", Email: "sari@tim.id", Role: base.RoleDeveloper, IsActive: true, PasswordHash: "x"}
	res, err := NewUsecase(&fakeRepo{user: u, found: true}).Execute(context.Background(), uid)
	want := getuser.Response{Found: true, ID: uid, Name: "Sari", Email: "sari@tim.id", Role: "developer", IsActive: true}
	if err != nil || res != want {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_NotFound(t *testing.T) {
	res, err := NewUsecase(&fakeRepo{}).Execute(context.Background(), uid)
	if err != nil || res != (getuser.Response{}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_InactiveUserStillFound(t *testing.T) {
	res, err := NewUsecase(&fakeRepo{user: base.User{ID: uid, IsActive: false}, found: true}).Execute(context.Background(), uid)
	if err != nil || !res.Found || res.IsActive {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_SuperAdminRole(t *testing.T) {
	res, _ := NewUsecase(&fakeRepo{user: base.User{ID: uid, Role: base.RoleSuperAdmin}, found: true}).Execute(context.Background(), uid)
	if res.Role != "super_admin" {
		t.Fatalf("%+v", res)
	}
}

func TestUsecase_Execute_RepoError(t *testing.T) {
	boom := errors.New("db down")
	res, err := NewUsecase(&fakeRepo{err: boom}).Execute(context.Background(), uid)
	if !errors.Is(err, boom) || res != (getuser.Response{}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_TrimsID(t *testing.T) {
	r := &fakeRepo{}
	_, _ = NewUsecase(r).Execute(context.Background(), " "+uid+" ")
	if r.got != uid {
		t.Fatalf("got %q", r.got)
	}
}

func TestUsecase_Execute_EmptyID(t *testing.T) {
	r := &fakeRepo{}
	res, err := NewUsecase(r).Execute(context.Background(), "")
	if err != nil || res.Found || r.got != "" {
		t.Fatalf("%+v %v", res, err)
	}
}
