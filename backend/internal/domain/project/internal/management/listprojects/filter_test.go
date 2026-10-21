package listprojects

import (
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
)

func ip(i int) *int { return &i }

func fields(t *testing.T, err error) base.FieldErrors {
	t.Helper()
	var fe base.FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("bukan FieldErrors: %v", err)
	}
	return fe
}

func TestNewProjectFilter_Defaults(t *testing.T) {
	f, err := NewProjectFilter(ProjectFilterInput{})
	if err != nil || f.Limit != 20 || f.Offset != 0 || f.Query != "" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestNewProjectFilter_LimitBounds(t *testing.T) {
	for _, n := range []int{1, 100} {
		if f, err := NewProjectFilter(ProjectFilterInput{Limit: ip(n)}); err != nil || f.Limit != n {
			t.Fatalf("%d: %v", n, err)
		}
	}
	for _, n := range []int{0, -1, 101, 500} {
		_, err := NewProjectFilter(ProjectFilterInput{Limit: ip(n)})
		if fe := fields(t, err); fe["limit"] != "limit harus 1-100" {
			t.Fatalf("%d: %v", n, fe)
		}
	}
}

func TestNewProjectFilter_OffsetBounds(t *testing.T) {
	if _, err := NewProjectFilter(ProjectFilterInput{Offset: ip(0)}); err != nil {
		t.Fatal(err)
	}
	_, err := NewProjectFilter(ProjectFilterInput{Offset: ip(-1)})
	if fields(t, err)["offset"] != "offset minimal 0" {
		t.Fatal(err)
	}
	_, err = NewProjectFilter(ProjectFilterInput{Offset: ip(1_000_001)})
	if fields(t, err)["offset"] != "offset terlalu besar" {
		t.Fatal(err)
	}
}

func TestNewProjectFilter_Query(t *testing.T) {
	f, _ := NewProjectFilter(ProjectFilterInput{Query: "  pay  "})
	if f.Query != "pay" {
		t.Fatal(f.Query)
	}
	f, _ = NewProjectFilter(ProjectFilterInput{Query: "   "})
	if f.Query != "" {
		t.Fatal(f.Query)
	}
	if _, err := NewProjectFilter(ProjectFilterInput{Query: strings.Repeat("é", 100)}); err != nil {
		t.Fatal(err)
	}
	_, err := NewProjectFilter(ProjectFilterInput{Query: strings.Repeat("é", 101)})
	if fields(t, err)["q"] != "maksimal 100 karakter" {
		t.Fatal(err)
	}
	for _, q := range []string{"a\nb", "a\x00b"} {
		_, err := NewProjectFilter(ProjectFilterInput{Query: q})
		if fields(t, err)["q"] != "mengandung karakter tidak valid" {
			t.Fatalf("%q: %v", q, err)
		}
	}
}

func TestNewProjectFilter_AggregatesErrors(t *testing.T) {
	_, err := NewProjectFilter(ProjectFilterInput{Query: strings.Repeat("a", 101), Limit: ip(999), Offset: ip(-1)})
	if fe := fields(t, err); len(fe) != 3 {
		t.Fatal(fe)
	}
}

func TestProjectFilter_LikePattern(t *testing.T) {
	if got := (ProjectFilter{Query: `50%_a\`}).LikePattern(); got != `%50\%\_a\\%` {
		t.Fatal(got)
	}
	if (ProjectFilter{}).LikePattern() != "" {
		t.Fatal("harus kosong")
	}
}
