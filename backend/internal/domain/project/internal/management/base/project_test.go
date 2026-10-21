package base

import (
	"errors"
	"strings"
	"testing"
	"time"

	"grimoire/lib/common/errs"
)

var t0 = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan *errs.Error: %v", err)
	}
	m, _ := e.Details.(map[string]any)
	fe, _ := m["fields"].(FieldErrors)
	return fe
}

func TestValidateName(t *testing.T) {
	cases := []struct {
		in, out string
		ok      bool
	}{
		{"A", "A", false},
		{"AB", "AB", true},
		{strings.Repeat("é", 100), strings.Repeat("é", 100), true},
		{strings.Repeat("é", 101), strings.Repeat("é", 101), false},
		{"  Nama  ", "Nama", true},
		{"a\nb", "a\nb", false},
		{"    ", "", false},
	}
	for _, c := range cases {
		got, fe := ValidateName(c.in)
		if (len(fe) == 0) != c.ok || got != c.out {
			t.Errorf("ValidateName(%q) = %q, %v", c.in, got, fe)
		}
	}
}

func TestValidateDescription(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{strings.Repeat("a", 1000), true},
		{strings.Repeat("a", 1001), false},
		{"a\nb\tc", true},
		{"a\x00b", false},
		{"", true},
	}
	for _, c := range cases {
		if _, fe := ValidateDescription(c.in); (len(fe) == 0) != c.ok {
			t.Errorf("ValidateDescription(%q) fe=%v", c.in, fe)
		}
	}
}

func TestNewProject(t *testing.T) {
	_, err := NewProject("x", strings.Repeat("a", 1001), Actor{ID: "u", Label: "u@x"}, t0)
	fe := fieldsOf(t, err)
	if fe["name"] == "" || fe["description"] == "" {
		t.Fatalf("fields = %v", fe)
	}
	p, err := NewProject(" Payments ", "", Actor{ID: "u", Label: "u@x"}, t0)
	if err != nil || p.Name != "Payments" || *p.CreatedBy != "u" || *p.CreatedByLabel != "u@x" || !p.UpdatedAt.Equal(t0) {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}

func TestApplyUpdate(t *testing.T) {
	later := t0.Add(time.Hour)
	str := func(s string) *string { return &s }
	p := Project{Name: "Alpha", Description: "d", UpdatedAt: t0}

	if _, err := p.ApplyUpdate(ProjectPatch{}, later); fieldsOf(t, err)["body"] == "" {
		t.Fatal("patch kosong harus error body")
	}
	ch, err := p.ApplyUpdate(ProjectPatch{Name: str(" Alpha ")}, later)
	if err != nil || len(ch) != 0 || !p.UpdatedAt.Equal(t0) {
		t.Fatalf("no-op: %v %v %v", ch, err, p.UpdatedAt)
	}
	if _, err := p.ApplyUpdate(ProjectPatch{Name: str("x")}, later); err == nil || p.Name != "Alpha" {
		t.Fatalf("nama invalid mengubah struct: %+v err=%v", p, err)
	}
	ch, err = p.ApplyUpdate(ProjectPatch{Name: str("Beta"), Description: str("e")}, later)
	if err != nil || len(ch) != 2 || ch[0] != "name" || ch[1] != "description" || !p.UpdatedAt.Equal(later) {
		t.Fatalf("ch=%v err=%v p=%+v", ch, err, p)
	}
}

func TestConfirmDeletion(t *testing.T) {
	p := Project{Name: "Payments"}
	if err := p.ConfirmDeletion("Payments"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"payments", "Payments ", ""} {
		if err := p.ConfirmDeletion(s); !errors.Is(err, ErrConfirmationMismatch) {
			t.Errorf("%q: %v", s, err)
		}
	}
}
