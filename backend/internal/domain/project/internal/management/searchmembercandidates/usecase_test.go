package searchmembercandidates

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/internal/infra/bus"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/finddevelopers"
	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/inviteuser"
	"grimoire/lib/contracts/core/auth/resendinvitation"
)

type fakeRepo struct {
	p            base.Project
	role         string
	found        bool
	findErr      error
	members      []base.Member
	membersErr   error
	findCalls    int
	membersCalls int
}

func (f *fakeRepo) FindAccess(context.Context, string, string) (base.Project, string, bool, error) {
	f.findCalls++
	return f.p, f.role, f.found, f.findErr
}

func (f *fakeRepo) ListMembers(context.Context, string) ([]base.Member, error) {
	f.membersCalls++
	return f.members, f.membersErr
}

type fakeUsers struct {
	devs      []finddevelopers.Developer
	err       error
	calls     int
	getCalls  int
	lastQ     string
	lastLimit int
}

func (f *fakeUsers) Get(context.Context, string) (getuser.Response, error) {
	f.getCalls++
	return getuser.Response{}, nil
}

func (f *fakeUsers) FindDevelopers(_ context.Context, q string, limit int) ([]finddevelopers.Developer, error) {
	f.calls++
	f.lastQ, f.lastLimit = q, limit
	return f.devs, f.err
}

func ctxFor(role string) context.Context {
	return reqctx.WithIdentity(context.Background(), reqctx.Identity{UserID: "u1", Role: role, Email: "u@x.id"})
}

func repoFound(role string, memberIDs ...string) *fakeRepo {
	r := &fakeRepo{p: base.Project{ID: "p1", Name: "P"}, role: role, found: true}
	for _, id := range memberIDs {
		r.members = append(r.members, base.Member{ProjectID: "p1", UserID: id})
	}
	return r
}

func devs(n int) []finddevelopers.Developer {
	out := make([]finddevelopers.Developer, n)
	for i := range out {
		out[i] = finddevelopers.Developer{ID: fmt.Sprintf("d%02d", i), Name: fmt.Sprintf("Dev %02d", i), Email: fmt.Sprintf("d%02d@x.id", i)}
	}
	return out
}

func exec(ctx context.Context, r *fakeRepo, u *fakeUsers, q, limit string) ([]base.CandidateDTO, error) {
	return NewUsecase(r, u).Execute(ctx, Input{ProjectID: "p1", RawQ: q, RawLimit: limit})
}

func TestUsecase_Execute_OwnerGetsCandidates(t *testing.T) {
	u := &fakeUsers{devs: devs(3)}
	out, err := exec(ctxFor("developer"), repoFound("owner", "d01"), u, "dev", "")
	if err != nil || len(out) != 2 || out[0].ID != "d00" || out[1].ID != "d02" {
		t.Fatalf("%+v %v", out, err)
	}
	if out[0].Name != "Dev 00" || out[0].Email != "d00@x.id" {
		t.Fatalf("%+v", out[0])
	}
}

func TestUsecase_Execute_SuperAdminAllowed(t *testing.T) {
	out, err := exec(ctxFor("super_admin"), repoFound(""), &fakeUsers{devs: devs(2)}, "d", "")
	if err != nil || len(out) != 2 {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestUsecase_Execute_MemberForbidden(t *testing.T) {
	r, u := repoFound("member"), &fakeUsers{devs: devs(2)}
	_, err := exec(ctxFor("developer"), r, u, "d", "")
	if !errors.Is(err, base.ErrForbidden) || r.membersCalls != 0 || u.calls != 0 {
		t.Fatalf("%v %d %d", err, r.membersCalls, u.calls)
	}
}

func TestUsecase_Execute_NonMemberIs404(t *testing.T) {
	r, u := repoFound(""), &fakeUsers{}
	_, err := exec(ctxFor("developer"), r, u, "d", "")
	if !errors.Is(err, base.ErrProjectNotFound) || r.membersCalls != 0 || u.calls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_Execute_NotFound(t *testing.T) {
	r := &fakeRepo{}
	_, err := exec(ctxFor("developer"), r, &fakeUsers{}, "d", "")
	if err != base.ErrProjectNotFound {
		t.Fatal(err)
	}
}

func TestUsecase_Execute_AuthBeforeValidation(t *testing.T) {
	if _, err := exec(ctxFor("developer"), repoFound(""), &fakeUsers{}, "d", "999"); !errors.Is(err, base.ErrProjectNotFound) {
		t.Fatalf("non-anggota: %v", err)
	}
	if _, err := exec(ctxFor("developer"), repoFound("member"), &fakeUsers{}, "d", "999"); !errors.Is(err, base.ErrForbidden) {
		t.Fatalf("member: %v", err)
	}
	_, err := exec(ctxFor("developer"), repoFound("owner"), &fakeUsers{}, "d", "999")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalid {
		t.Fatalf("owner: %v", err)
	}
}

func TestUsecase_Execute_EmptyQueryNoBus(t *testing.T) {
	for _, q := range []string{"", "   "} {
		r, u := repoFound("owner"), &fakeUsers{devs: devs(3)}
		out, err := exec(ctxFor("developer"), r, u, q, "")
		if err != nil || out == nil || len(out) != 0 || r.membersCalls != 0 || u.calls != 0 {
			t.Fatalf("%q: %#v %v", q, out, err)
		}
	}
}

func TestUsecase_Execute_PassesTrimmedQueryAndMax50(t *testing.T) {
	u := &fakeUsers{}
	if _, err := exec(ctxFor("developer"), repoFound("owner"), u, "  bud ", "3"); err != nil {
		t.Fatal(err)
	}
	if u.lastQ != "bud" || u.lastLimit != 50 {
		t.Fatalf("%q %d", u.lastQ, u.lastLimit)
	}
}

func TestUsecase_Execute_TruncatesToLimit(t *testing.T) {
	out, err := exec(ctxFor("developer"), repoFound("owner"), &fakeUsers{devs: devs(50)}, "d", "5")
	if err != nil || len(out) != 5 || out[4].ID != "d04" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestUsecase_Execute_DefaultLimit10(t *testing.T) {
	out, err := exec(ctxFor("developer"), repoFound("owner"), &fakeUsers{devs: devs(50)}, "d", "")
	if err != nil || len(out) != 10 {
		t.Fatalf("%d %v", len(out), err)
	}
}

func TestUsecase_Execute_FiltersMembersBeforeTruncate(t *testing.T) {
	r := repoFound("owner", "d00", "d01", "d02", "d03", "d04")
	out, err := exec(ctxFor("developer"), r, &fakeUsers{devs: devs(10)}, "d", "3")
	if err != nil || len(out) != 3 || out[0].ID != "d05" || out[2].ID != "d07" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestUsecase_Execute_AllMembersReturnsEmpty(t *testing.T) {
	r := repoFound("owner", "d00", "d01")
	out, err := exec(ctxFor("developer"), r, &fakeUsers{devs: devs(2)}, "d", "")
	if err != nil || out == nil || len(out) != 0 {
		t.Fatalf("%#v %v", out, err)
	}
}

func TestUsecase_Execute_MapsOnlyKnownFields(t *testing.T) {
	if n := reflect.TypeOf(base.CandidateDTO{}).NumField(); n != 4 { // id, name, email, invitation_pending (T12)
		t.Fatalf("fields %d", n)
	}
}

func TestUsecase_Execute_NoIdentity(t *testing.T) {
	r := repoFound("owner")
	_, err := exec(context.Background(), r, &fakeUsers{}, "d", "")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUnauthorized || r.findCalls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_Execute_FindAccessError(t *testing.T) {
	boom := errors.New("boom")
	r := &fakeRepo{findErr: boom}
	_, err := exec(ctxFor("developer"), r, &fakeUsers{}, "d", "")
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_Execute_ListMembersError(t *testing.T) {
	boom := errors.New("boom")
	r, u := repoFound("owner"), &fakeUsers{}
	r.membersErr = boom
	_, err := exec(ctxFor("developer"), r, u, "d", "")
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) || u.calls != 0 {
		t.Fatalf("%v", err)
	}
}

func TestUsecase_Execute_FindDevelopersError(t *testing.T) {
	u := &fakeUsers{err: fmt.Errorf("users: find-developers: %w", bus.ErrNotRegistered)}
	out, err := exec(ctxFor("developer"), repoFound("owner"), u, "d", "")
	var e *errs.Error
	if !errors.Is(err, bus.ErrNotRegistered) || errors.As(err, &e) || out != nil {
		t.Fatalf("%v %v", out, err)
	}
}

func TestUsecase_Execute_NoGetUser(t *testing.T) {
	u := &fakeUsers{devs: devs(2)}
	if _, err := exec(ctxFor("developer"), repoFound("owner"), u, "d", ""); err != nil || u.getCalls != 0 {
		t.Fatalf("%v %d", err, u.getCalls)
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

func TestUsecase_Execute_MapsInvitationPending(t *testing.T) {
	ds := devs(2)
	ds[1].InvitationPending = true
	out, err := exec(ctxFor("developer"), repoFound("owner"), &fakeUsers{devs: ds}, "d", "")
	if err != nil || len(out) != 2 {
		t.Fatalf("%#v %v", out, err)
	}
	if out[0].InvitationPending || !out[1].InvitationPending {
		t.Fatalf("%#v", out)
	}
}
