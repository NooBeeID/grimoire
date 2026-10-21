package countspecs

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

type fakeCounter struct {
	calls int
	got   []string
	ret   map[string]int
	err   error
}

func (f *fakeCounter) CountByProjects(_ context.Context, ids []string) (map[string]int, error) {
	f.calls++
	f.got = ids
	return f.ret, f.err
}

var (
	idA = "0b6c2e1a-7d4f-4c1e-9a55-3f2b8c9d1e10"
	idB = "9a1f4c0e-2b7d-4e63-8f10-5c9d3a7e1b22"
	idC = "11111111-2222-4333-8444-555555555555"
)

func exec(f *fakeCounter, ids []string) (contentcount.Response, error) {
	return NewUsecase(f).Execute(context.Background(), contentcount.Request{ProjectIDs: ids})
}

func manyIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = uuid.NewString()
	}
	return out
}

func TestExecute_BatchSeveralProjects(t *testing.T) {
	f := &fakeCounter{ret: map[string]int{idA: 3, idB: 1}}
	res, err := exec(f, []string{idA, idB, idC})
	if err != nil || !reflect.DeepEqual(res.Counts, map[string]int{idA: 3, idB: 1}) {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if f.calls != 1 || !reflect.DeepEqual(f.got, []string{idA, idB, idC}) {
		t.Fatalf("calls=%d got=%v", f.calls, f.got)
	}
}

func TestExecute_InvalidIDsIgnored(t *testing.T) {
	f := &fakeCounter{ret: map[string]int{}}
	if _, err := exec(f, []string{idA, "x", "", "123", idB}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got, []string{idA, idB}) {
		t.Fatalf("got=%v", f.got)
	}
}

func TestExecute_AllInvalidSkipsQuery(t *testing.T) {
	f := &fakeCounter{}
	res, err := exec(f, []string{"x", "y"})
	if err != nil || res.Counts == nil || len(res.Counts) != 0 || f.calls != 0 {
		t.Fatalf("res=%v err=%v calls=%d", res, err, f.calls)
	}
}

func TestExecute_EmptyAndNilRequest(t *testing.T) {
	for _, in := range [][]string{nil, {}} {
		f := &fakeCounter{}
		res, err := exec(f, in)
		if err != nil || res.Counts == nil || len(res.Counts) != 0 || f.calls != 0 {
			t.Fatalf("res=%v err=%v calls=%d", res, err, f.calls)
		}
	}
}

func TestExecute_DedupAndCanonicalCase(t *testing.T) {
	f := &fakeCounter{}
	if _, err := exec(f, []string{idA, idA, strings.ToUpper(idA)}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got, []string{idA}) {
		t.Fatalf("got=%v", f.got)
	}
}

func TestExecute_LimitExactlyMaxAccepted(t *testing.T) {
	f := &fakeCounter{}
	if _, err := exec(f, manyIDs(contentcount.MaxProjectIDs)); err != nil || f.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
}

func TestExecute_OverLimitRejected(t *testing.T) {
	f := &fakeCounter{}
	ids := append(manyIDs(100), "bukan-uuid")
	_, err := exec(f, ids)
	var e *errs.Error
	if !errors.Is(err, errs.ErrInvalid) || !errors.As(err, &e) || e.Code != errs.CodeInvalid || f.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
}

func TestExecute_LimitCountsRawLength(t *testing.T) {
	f := &fakeCounter{}
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = idA
	}
	if _, err := exec(f, ids); !errors.Is(err, errs.ErrInvalid) || f.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
}

func TestExecute_RepoErrorWrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := exec(&fakeCounter{err: boom}, []string{idA})
	var e *errs.Error
	if !errors.Is(err, boom) || errors.As(err, &e) || !strings.HasPrefix(err.Error(), "countspecs: count by projects") {
		t.Fatalf("err=%v", err)
	}
}

func TestExecute_NilCountsFromRepoBecomesEmptyMap(t *testing.T) {
	res, err := exec(&fakeCounter{ret: nil}, []string{idA})
	if err != nil || res.Counts == nil || len(res.Counts) != 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestNormalizeIDs_Format(t *testing.T) {
	got := normalizeIDs([]string{fmt.Sprintf("  %s", idA), idB})
	if !reflect.DeepEqual(got, []string{idB}) {
		t.Fatalf("got=%v", got)
	}
}
