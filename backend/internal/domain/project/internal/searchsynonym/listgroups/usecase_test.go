package listgroups

import (
	"context"
	"errors"
	"testing"
	"time"

	"grimoire/internal/domain/project/internal/searchsynonym/base"
)

type fakeAccess struct{ err error }

func (f fakeAccess) Require(context.Context, string) (base.Actor, error) {
	return base.Actor{ID: "u", Label: "Budi"}, f.err
}

type fakeRepo struct {
	groups []base.Group
	err    error
	calls  int
}

func (f *fakeRepo) List(context.Context, string) ([]base.Group, error) {
	f.calls++
	return f.groups, f.err
}

func TestExecute_Empty(t *testing.T) {
	got, err := NewUsecase(fakeAccess{}, &fakeRepo{}).Execute(context.Background(), "p")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v %v", got, err)
	}
}

func TestExecute_Maps(t *testing.T) {
	now := time.Now().UTC()
	r := &fakeRepo{groups: []base.Group{{ID: "g1", Terms: []string{"a", "b"}, CreatedAt: now, CreatedByLabel: "Budi"}}}
	got, err := NewUsecase(fakeAccess{}, r).Execute(context.Background(), "p")
	if err != nil || len(got) != 1 || got[0].ID != "g1" || got[0].CreatedByLabel != "Budi" {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestExecute_NotMember(t *testing.T) {
	r := &fakeRepo{}
	_, err := NewUsecase(fakeAccess{err: base.ErrProjectNotFound}, r).Execute(context.Background(), "p")
	if !errors.Is(err, base.ErrProjectNotFound) || r.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, r.calls)
	}
}

func TestExecute_RepoError(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewUsecase(fakeAccess{}, &fakeRepo{err: boom}).Execute(context.Background(), "p")
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}
