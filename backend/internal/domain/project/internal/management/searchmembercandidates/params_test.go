package searchmembercandidates

import (
	"errors"
	"strings"
	"testing"

	"grimoire/internal/domain/project/internal/management/base"
	"grimoire/lib/common/errs"
)

func fieldsOf(t *testing.T, err error) base.FieldErrors {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("bukan errs.Error: %v", err)
	}
	d, _ := e.Details.(map[string]any)
	f, _ := d["fields"].(base.FieldErrors)
	return f
}

func TestParseQuery_Defaults(t *testing.T) {
	q, err := parseQuery("", "")
	if err != nil || q.Q != "" || q.Limit != 10 {
		t.Fatalf("%+v %v", q, err)
	}
	q, err = parseQuery("  bud  ", "")
	if err != nil || q.Q != "bud" {
		t.Fatalf("%+v %v", q, err)
	}
}

func TestParseQuery_LimitBoundaries(t *testing.T) {
	for _, s := range []string{"1", "50"} {
		if _, err := parseQuery("a", s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"0", "51", "-1", "abc", "1.5", "10x"} {
		_, err := parseQuery("a", s)
		if err == nil || fieldsOf(t, err)["limit"] != "limit harus 1-50" {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestParseQuery_QLength(t *testing.T) {
	if _, err := parseQuery(strings.Repeat("é", 100), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := parseQuery(" "+strings.Repeat("a", 100)+"  ", ""); err != nil {
		t.Fatal(err)
	}
	_, err := parseQuery(strings.Repeat("é", 101), "")
	if err == nil || fieldsOf(t, err)["q"] != "q maksimal 100 karakter" {
		t.Fatal(err)
	}
}

func TestParseQuery_BothInvalidMerged(t *testing.T) {
	_, err := parseQuery(strings.Repeat("a", 101), "0")
	f := fieldsOf(t, err)
	if f["q"] == "" || f["limit"] == "" {
		t.Fatalf("%v", f)
	}
}

func TestParseQuery_SpaceOnlyLimitIsDefault(t *testing.T) {
	q, err := parseQuery("a", "  ")
	if err != nil || q.Limit != 10 {
		t.Fatalf("%+v %v", q, err)
	}
}
