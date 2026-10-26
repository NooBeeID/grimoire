package buslistgroups

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/searchsynonym/listgroups"
)

type fakeRepo struct {
	groups [][]string
	err    error
	called int
	gotPID string
}

func (f *fakeRepo) ListGroups(_ context.Context, pid string) ([][]string, error) {
	f.called++
	f.gotPID = pid
	return f.groups, f.err
}

func TestExecute_InvalidProject(t *testing.T) {
	repo := &fakeRepo{}
	_, err := New(repo).Execute(context.Background(), listgroups.Request{ProjectID: "x"})
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
	if repo.called != 0 {
		t.Fatal("repo dipanggil")
	}
}

func TestExecute_Groups(t *testing.T) {
	pid := uuid.NewString()
	repo := &fakeRepo{groups: [][]string{{"login", "autentikasi"}, {"refund", "pengembalian dana"}}}
	res, err := New(repo).Execute(context.Background(), listgroups.Request{ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Groups, repo.groups) || repo.gotPID != pid {
		t.Fatalf("%+v pid=%s", res, repo.gotPID)
	}
}

func TestExecute_EmptyIsNil(t *testing.T) {
	res, err := New(&fakeRepo{groups: [][]string{}}).Execute(context.Background(), listgroups.Request{ProjectID: uuid.NewString()})
	if err != nil || res.Groups != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestExecute_RepoError(t *testing.T) {
	boom := errors.New("boom")
	_, err := New(&fakeRepo{err: boom}).Execute(context.Background(), listgroups.Request{ProjectID: uuid.NewString()})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
