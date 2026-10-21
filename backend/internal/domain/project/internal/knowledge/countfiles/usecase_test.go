package countfiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"grimoire/lib/common/errs"
)

type fakeRepo struct {
	calls  [][]string
	result map[string]int
	err    error
}

func (f *fakeRepo) CountByProject(_ context.Context, ids []string) (map[string]int, error) {
	f.calls = append(f.calls, append([]string(nil), ids...))
	return f.result, f.err
}

func newID() string { return uuid.NewString() }

func TestCountBatch(t *testing.T) {
	a, b, c := newID(), newID(), newID()
	repo := &fakeRepo{result: map[string]int{a: 12, b: 3}}
	got, err := NewUsecase(repo).Count(context.Background(), []string{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[a] != 12 || got[b] != 3 {
		t.Fatalf("got %v", got)
	}
	if v, ok := got[c]; !ok || v != 0 {
		t.Fatalf("c harus ada dengan 0: %v", got)
	}
	if len(repo.calls) != 1 || len(repo.calls[0]) != 3 {
		t.Fatalf("calls %v", repo.calls)
	}
}

func TestCountInvalidIDsNotQueried(t *testing.T) {
	a := newID()
	repo := &fakeRepo{result: map[string]int{a: 2}}
	got, err := NewUsecase(repo).Count(context.Background(), []string{"bukan-uuid", "", "123", a})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"bukan-uuid", "", "123"} {
		if v, ok := got[k]; !ok || v != 0 {
			t.Fatalf("%q: %v", k, got)
		}
	}
	if got[a] != 2 || len(repo.calls) != 1 || len(repo.calls[0]) != 1 || repo.calls[0][0] != a {
		t.Fatalf("got %v calls %v", got, repo.calls)
	}
}

func TestCountNoQueryWhenNoValidIDs(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"x", "y"}} {
		repo := &fakeRepo{}
		got, err := NewUsecase(repo).Count(context.Background(), in)
		if err != nil || got == nil || len(got) != len(in) {
			t.Fatalf("in %v: got %v err %v", in, got, err)
		}
		for _, v := range got {
			if v != 0 {
				t.Fatalf("nonzero %v", got)
			}
		}
		if len(repo.calls) != 0 {
			t.Fatalf("repo dipanggil: %v", repo.calls)
		}
	}
}

func TestCountDuplicates(t *testing.T) {
	a := newID()
	repo := &fakeRepo{result: map[string]int{a: 5}}
	got, err := NewUsecase(repo).Count(context.Background(), []string{a, a})
	if err != nil || got[a] != 5 || len(got) != 1 {
		t.Fatalf("got %v err %v", got, err)
	}
	if len(repo.calls[0]) != 1 {
		t.Fatalf("calls %v", repo.calls)
	}
}

func TestCountUppercaseUUID(t *testing.T) {
	a := newID()
	up := strings.ToUpper(a)
	repo := &fakeRepo{result: map[string]int{a: 7}}
	got, err := NewUsecase(repo).Count(context.Background(), []string{a, up})
	if err != nil {
		t.Fatal(err)
	}
	if got[a] != 7 || got[up] != 7 {
		t.Fatalf("got %v", got)
	}
	if len(repo.calls) != 1 || len(repo.calls[0]) != 1 || repo.calls[0][0] != a {
		t.Fatalf("calls %v", repo.calls)
	}
}

func TestCountLimit(t *testing.T) {
	mk := func(n int) []string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = newID()
		}
		return ids
	}
	repo := &fakeRepo{}
	if _, err := NewUsecase(repo).Count(context.Background(), mk(100)); err != nil {
		t.Fatalf("100 harus ok: %v", err)
	}
	repo = &fakeRepo{}
	_, err := NewUsecase(repo).Count(context.Background(), mk(101))
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	if len(repo.calls) != 0 {
		t.Fatal("repo tidak boleh dipanggil")
	}
}

func TestCountRepoError(t *testing.T) {
	sentinel := errors.New("boom")
	got, err := NewUsecase(&fakeRepo{err: sentinel}).Count(context.Background(), []string{newID()})
	if !errors.Is(err, sentinel) || got != nil {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestCountIgnoresUnrequestedCanon(t *testing.T) {
	a, other := newID(), newID()
	repo := &fakeRepo{result: map[string]int{a: 1, other: 99}}
	got, err := NewUsecase(repo).Count(context.Background(), []string{a})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[other]; ok || len(got) != 1 {
		t.Fatalf("bocor: %v", fmt.Sprint(got))
	}
}
