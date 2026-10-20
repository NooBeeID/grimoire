package finddevelopers

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"grimoire/internal/domain/core/internal/auth/base"
	contract "grimoire/lib/contracts/core/auth/finddevelopers"
)

type fakeRepo struct {
	calls   int
	pattern string
	limit   int
	items   []contract.Developer
	err     error
}

func (f *fakeRepo) FindActiveDevelopers(_ context.Context, p string, l int) ([]contract.Developer, error) {
	f.calls++
	f.pattern, f.limit = p, l
	return f.items, f.err
}

func TestNormalizeLimit(t *testing.T) {
	for in, want := range map[int]int{-5: 10, 0: 10, 1: 1, 10: 10, 50: 50, 51: 50, 1000000: 50} {
		if got := normalizeLimit(in); got != want {
			t.Errorf("normalizeLimit(%d)=%d want %d", in, got, want)
		}
	}
}

func TestNormalizeQuery(t *testing.T) {
	for in, want := range map[string]string{"  sar  ": "sar", "": "", "   ": "", "\t\n": ""} {
		if got := normalizeQuery(in); got != want {
			t.Errorf("normalizeQuery(%q)=%q want %q", in, got, want)
		}
	}
	if got := normalizeQuery(strings.Repeat("a", 150)); len(got) != 100 {
		t.Errorf("len=%d", len(got))
	}
	got := normalizeQuery(strings.Repeat("é", 150))
	if utf8.RuneCountInString(got) != 100 || !utf8.ValidString(got) {
		t.Errorf("multibyte: runes=%d valid=%v", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	in := strings.Repeat("a", 99) + " " + strings.Repeat("b", 20)
	if got := normalizeQuery(in); got != strings.Repeat("a", 99) {
		t.Errorf("trailing space not trimmed: %q", got)
	}
}

func TestUsecase_Execute_EmptyQueryNoRepoCall(t *testing.T) {
	for _, q := range []string{"", "   "} {
		r := &fakeRepo{}
		res, err := NewUsecase(r).Execute(context.Background(), contract.Request{Query: q})
		if err != nil || res.Items == nil || len(res.Items) != 0 || r.calls != 0 {
			t.Fatalf("q=%q res=%+v err=%v calls=%d", q, res, err, r.calls)
		}
	}
}

func TestUsecase_Execute_PassesEscapedPattern(t *testing.T) {
	r := &fakeRepo{}
	q := `50%_a\`
	if _, err := NewUsecase(r).Execute(context.Background(), contract.Request{Query: q}); err != nil {
		t.Fatal(err)
	}
	if r.pattern != base.LikePattern(q) {
		t.Fatalf("pattern=%q", r.pattern)
	}
}

func TestUsecase_Execute_TrimsQuery(t *testing.T) {
	r := &fakeRepo{}
	_, _ = NewUsecase(r).Execute(context.Background(), contract.Request{Query: " sar "})
	if r.pattern != base.LikePattern("sar") {
		t.Fatalf("pattern=%q", r.pattern)
	}
}

func TestUsecase_Execute_Limits(t *testing.T) {
	for in, want := range map[int]int{0: 10, 500: 50, 7: 7} {
		r := &fakeRepo{}
		_, _ = NewUsecase(r).Execute(context.Background(), contract.Request{Query: "x", Limit: in})
		if r.limit != want {
			t.Errorf("limit %d -> %d want %d", in, r.limit, want)
		}
	}
}

func TestUsecase_Execute_MapsItems(t *testing.T) {
	items := []contract.Developer{{ID: "1", Name: "A", Email: "a@x"}, {ID: "2", Name: "B", Email: "b@x"}}
	res, err := NewUsecase(&fakeRepo{items: items}).Execute(context.Background(), contract.Request{Query: "x"})
	if err != nil || len(res.Items) != 2 || res.Items[0] != items[0] || res.Items[1] != items[1] {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_NilFromRepoBecomesEmptySlice(t *testing.T) {
	res, err := NewUsecase(&fakeRepo{}).Execute(context.Background(), contract.Request{Query: "x"})
	if err != nil || res.Items == nil || len(res.Items) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestUsecase_Execute_RepoError(t *testing.T) {
	boom := errors.New("boom")
	res, err := NewUsecase(&fakeRepo{err: boom}).Execute(context.Background(), contract.Request{Query: "x"})
	if !errors.Is(err, boom) || res.Items != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestDeveloperFields(t *testing.T) {
	if n := reflectNumField(contract.Developer{}); n != 3 {
		t.Fatalf("fields=%d", n)
	}
}

func reflectNumField(v any) int { return reflect.TypeOf(v).NumField() }
