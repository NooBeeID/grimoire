package checkaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
	"grimoire/lib/contracts/project/management/checkaccess"
)

type fakeAccess struct {
	role  string
	found bool
	err   error
	calls int
}

func (f *fakeAccess) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	f.calls++
	return base.Project{}, f.role, f.found, f.err
}

type fakeUsers struct {
	res   getuser.Response
	err   error
	calls int
	last  string
}

func (f *fakeUsers) Get(_ context.Context, id string) (getuser.Response, error) {
	f.calls++
	f.last = id
	return f.res, f.err
}
func (f *fakeUsers) FindDevelopers(context.Context, string, int) ([]finddevelopers.Developer, error) {
	return nil, nil
}

func req(role string) checkaccess.Request {
	return checkaccess.Request{ProjectID: uuid.NewString(), UserID: uuid.NewString(), UserRole: role}
}

func TestExecute_SuperAdminGetsAdminWithoutGetUser(t *testing.T) {
	for _, role := range []string{"", "owner", "member"} {
		a, u := &fakeAccess{role: role, found: true}, &fakeUsers{}
		res, err := NewUsecase(a, u).Execute(context.Background(), req("super_admin"))
		if err != nil || res != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelAdmin}) || u.calls != 0 {
			t.Fatalf("role=%q res=%+v err=%v calls=%d", role, res, err, u.calls)
		}
	}
}

func TestExecute_DeveloperNotMemberIsNone(t *testing.T) {
	u := &fakeUsers{}
	res, err := NewUsecase(&fakeAccess{found: true}, u).Execute(context.Background(), req("developer"))
	if err != nil || res != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}) || u.calls != 0 {
		t.Fatalf("%+v %v %d", res, err, u.calls)
	}
}

func TestExecute_DeveloperOwnerAndMember(t *testing.T) {
	for role, want := range map[string]checkaccess.Level{"owner": checkaccess.LevelOwner, "member": checkaccess.LevelMember} {
		u := &fakeUsers{res: getuser.Response{Found: true, IsActive: true}}
		r := req("developer")
		res, err := NewUsecase(&fakeAccess{role: role, found: true}, u).Execute(context.Background(), r)
		if err != nil || res.Level != want || !res.ProjectExists || u.calls != 1 || u.last != r.UserID {
			t.Fatalf("%s: %+v %v calls=%d last=%s", role, res, err, u.calls, u.last)
		}
	}
}

func TestT03_AC5_DeactivatedMemberIsNone(t *testing.T) {
	u := &fakeUsers{res: getuser.Response{Found: true, IsActive: false}}
	res, err := NewUsecase(&fakeAccess{role: "member", found: true}, u).Execute(context.Background(), req("developer"))
	if err != nil || res != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestT03_AC5_MemberNotFoundInAuthIsNone(t *testing.T) {
	u := &fakeUsers{res: getuser.Response{Found: false}}
	res, err := NewUsecase(&fakeAccess{role: "owner", found: true}, u).Execute(context.Background(), req("developer"))
	if err != nil || res != (checkaccess.Response{ProjectExists: true, Level: checkaccess.LevelNone}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestT03_AC8_ProjectNotFound(t *testing.T) {
	u := &fakeUsers{}
	res, err := NewUsecase(&fakeAccess{found: false}, u).Execute(context.Background(), req("developer"))
	if err != nil || res != (checkaccess.Response{ProjectExists: false, Level: checkaccess.LevelNone}) || u.calls != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_GetUserErrorIsFailClosed(t *testing.T) {
	for _, cause := range []error{errors.New("boom"), bus.ErrNotRegistered} {
		u := &fakeUsers{err: cause}
		res, err := NewUsecase(&fakeAccess{role: "owner", found: true}, u).Execute(context.Background(), req("developer"))
		if err == nil || !errors.Is(err, cause) || res != (checkaccess.Response{}) {
			t.Fatalf("%+v %v", res, err)
		}
	}
}

func TestExecute_FindAccessErrorIsWrapped(t *testing.T) {
	cause := errors.New("db down")
	res, err := NewUsecase(&fakeAccess{err: cause}, &fakeUsers{}).Execute(context.Background(), req("developer"))
	if !errors.Is(err, cause) || errors.Is(err, errs.ErrInvalid) || res != (checkaccess.Response{}) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_InvalidInput(t *testing.T) {
	cases := []checkaccess.Request{
		{ProjectID: uuid.NewString(), UserID: "", UserRole: "developer"},
		{ProjectID: uuid.NewString(), UserID: "bukan-uuid", UserRole: "developer"},
		{ProjectID: uuid.NewString(), UserID: uuid.NewString(), UserRole: ""},
		{ProjectID: uuid.NewString(), UserID: uuid.NewString(), UserRole: "agent"},
		{ProjectID: uuid.NewString(), UserID: uuid.NewString(), UserRole: "admin"},
	}
	for _, c := range cases {
		a := &fakeAccess{found: true}
		_, err := NewUsecase(a, &fakeUsers{}).Execute(context.Background(), c)
		if !errors.Is(err, errs.ErrInvalid) || a.calls != 0 {
			t.Fatalf("%+v: err=%v calls=%d", c, err, a.calls)
		}
	}
}

func TestExecute_ProjectIDNotUUIDPassedToStore(t *testing.T) {
	a := &fakeAccess{found: false}
	r := req("developer")
	r.ProjectID = "bukan-uuid"
	res, err := NewUsecase(a, &fakeUsers{}).Execute(context.Background(), r)
	if err != nil || a.calls != 1 || res != (checkaccess.Response{ProjectExists: false, Level: checkaccess.LevelNone}) {
		t.Fatalf("%+v %v %d", res, err, a.calls)
	}
}

func TestExecute_RoleFromMembershipUnknownIsNone(t *testing.T) {
	u := &fakeUsers{res: getuser.Response{Found: true, IsActive: true}}
	res, err := NewUsecase(&fakeAccess{role: "aneh", found: true}, u).Execute(context.Background(), req("developer"))
	if err != nil || res.Level != checkaccess.LevelNone {
		t.Fatalf("%+v %v", res, err)
	}
}

// Stub T12: tidak dipakai feature ini.
func (f *fakeUsers) GetByEmail(context.Context, string) (getuser.Response, error) {
	return getuser.Response{}, nil
}

func (f *fakeUsers) Invite(context.Context, inviteuser.Request) (inviteuser.Response, error) {
	return inviteuser.Response{}, nil
}

func (f *fakeUsers) ResendInvitation(context.Context, resendinvitation.Request) (resendinvitation.Response, error) {
	return resendinvitation.Response{}, nil
}
