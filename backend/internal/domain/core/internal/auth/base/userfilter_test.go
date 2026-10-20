package base

import (
	"strings"
	"testing"
)

func ip(n int) *int { return &n }

func TestNewUserFilter_Defaults(t *testing.T) {
	f, err := NewUserFilter(UserFilterInput{})
	if err != nil || f.Limit != DefaultUserLimit || f.Offset != 0 || f.Role != "" || f.IsActive != nil || f.Query != "" {
		t.Fatalf("%+v %v", f, err)
	}
	if f.LikePattern() != "" {
		t.Fatal("tanpa query pola harus kosong")
	}
}

func TestNewUserFilter_Valid(t *testing.T) {
	f, err := NewUserFilter(UserFilterInput{Role: " developer ", Status: "active", Query: "  50%_ ", Limit: ip(100), Offset: ip(40)})
	if err != nil || f.Role != "developer" || f.IsActive == nil || !*f.IsActive || f.Query != "50%_" || f.Limit != 100 || f.Offset != 40 {
		t.Fatalf("%+v %v", f, err)
	}
	if f.LikePattern() != `%50\%\_%` {
		t.Fatal(f.LikePattern())
	}
	f, _ = NewUserFilter(UserFilterInput{Status: "inactive"})
	if f.IsActive == nil || *f.IsActive {
		t.Fatal("inactive")
	}
}

func TestNewUserFilter_AggregatesErrors(t *testing.T) {
	_, err := NewUserFilter(UserFilterInput{Role: "x", Status: "y", Limit: ip(999)})
	fe := AsFieldErrors(err)
	if len(fe) != 3 || fe["role"] == "" || fe["status"] != "harus active atau inactive" || fe["limit"] != "maksimal 100" {
		t.Fatalf("%v", fe)
	}
}

func TestNewUserFilter_Bounds(t *testing.T) {
	cases := []struct {
		in    UserFilterInput
		field string
		msg   string
	}{
		{UserFilterInput{Limit: ip(0)}, "limit", "minimal 1"},
		{UserFilterInput{Offset: ip(-1)}, "offset", "minimal 0"},
		{UserFilterInput{Offset: ip(1_000_001)}, "offset", "terlalu besar"},
		{UserFilterInput{Query: strings.Repeat("a", 101)}, "q", "maksimal 100 karakter"},
		{UserFilterInput{Query: "a\tb"}, "q", "mengandung karakter tidak valid"},
	}
	for _, c := range cases {
		_, err := NewUserFilter(c.in)
		if AsFieldErrors(err)[c.field] != c.msg {
			t.Errorf("%s: %v", c.field, err)
		}
	}
	if _, err := NewUserFilter(UserFilterInput{Limit: ip(1), Offset: ip(1_000_000), Query: strings.Repeat("é", 100)}); err != nil {
		t.Fatal(err)
	}
}
