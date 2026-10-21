package countkeys

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/project/management/contentcount"
)

type fakeRepo struct {
	calls  int
	got    []string
	result map[string]int
	err    error
}

func (f *fakeRepo) CountActiveByProjects(_ context.Context, ids []string) (map[string]int, error) {
	f.calls++
	f.got = ids
	return f.result, f.err
}

func newIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = uuid.NewString()
	}
	return out
}

func TestCount_EmptyRequest_NoQuery(t *testing.T) {
	for _, in := range [][]string{nil, {}} {
		f := &fakeRepo{}
		res, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: in})
		if err != nil || res.Counts == nil || len(res.Counts) != 0 || f.calls != 0 {
			t.Fatalf("res=%v err=%v calls=%d", res, err, f.calls)
		}
	}
}

func TestCount_AllInvalidIDs_NoQuery(t *testing.T) {
	f := &fakeRepo{}
	res, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: []string{"bukan-uuid", "", "123"}})
	if err != nil || res.Counts == nil || len(res.Counts) != 0 || f.calls != 0 {
		t.Fatalf("res=%v err=%v calls=%d", res, err, f.calls)
	}
}

func TestCount_TooManyIDs_Invalid(t *testing.T) {
	f := &fakeRepo{}
	_, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: newIDs(101)})
	var e *errs.Error
	if !errors.Is(err, errs.ErrInvalid) || !errors.As(err, &e) || e.Code != errs.CodeInvalid || f.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
	f2 := &fakeRepo{result: map[string]int{}}
	if _, err := NewUsecase(f2).Count(context.Background(), contentcount.Request{ProjectIDs: newIDs(100)}); err != nil || f2.calls != 1 {
		t.Fatalf("100 id harus sukses: err=%v calls=%d", err, f2.calls)
	}
}

func TestCount_LimitCheckedOnRawLength(t *testing.T) {
	f := &fakeRepo{}
	raw := make([]string, 101)
	for i := range raw {
		raw[i] = fmt.Sprintf("x%d", i)
	}
	_, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: raw})
	if !errors.Is(err, errs.ErrInvalid) || f.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
}

func TestCount_IgnoresInvalid_PassesOnlyValid(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	f := &fakeRepo{}
	_, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: []string{a, "bukan-uuid", b}})
	if err != nil || !reflect.DeepEqual(f.got, []string{a, b}) {
		t.Fatalf("err=%v got=%v", err, f.got)
	}
}

func TestCount_NormalizesAndDedupes(t *testing.T) {
	a := uuid.NewString()
	f := &fakeRepo{}
	in := []string{strings.ToUpper(a), "{" + a + "}", a}
	if _, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: in}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got, []string{a}) || f.calls != 1 {
		t.Fatalf("got=%v calls=%d", f.got, f.calls)
	}
}

func TestCount_PassesRepoCountsThrough(t *testing.T) {
	a := uuid.NewString()
	f := &fakeRepo{result: map[string]int{a: 2}}
	res, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: []string{a}})
	if err != nil || !reflect.DeepEqual(res.Counts, map[string]int{a: 2}) {
		t.Fatalf("res=%v err=%v", res, err)
	}
	f.result = nil
	res, err = NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: []string{a}})
	if err != nil || res.Counts == nil || len(res.Counts) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestCount_RepoError_Wrapped(t *testing.T) {
	sentinel := errors.New("db down")
	f := &fakeRepo{err: sentinel}
	_, err := NewUsecase(f).Count(context.Background(), contentcount.Request{ProjectIDs: []string{uuid.NewString()}})
	var e *errs.Error
	if !errors.Is(err, sentinel) || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "countkeys: count active") {
		t.Fatalf("err=%v", err)
	}
}
