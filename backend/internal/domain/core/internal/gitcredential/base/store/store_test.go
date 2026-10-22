package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestViolation(t *testing.T) {
	code, c, ok := Violation(errors.Join(errors.New("x"), &pgconn.PgError{Code: "23505", ConstraintName: ConstraintUnique}))
	if !ok || code != "23505" || c != ConstraintUnique {
		t.Fatal(code, c, ok)
	}
	if _, _, ok := Violation(&pgconn.PgError{Code: "42P01"}); ok {
		t.Fatal("bukan constraint")
	}
	if _, _, ok := Violation(errors.New("x")); ok {
		t.Fatal("bukan pg")
	}
	if !ValidUUID("0192f5a0-aaaa-7aaa-8bbb-00000000000a") || ValidUUID("zz") {
		t.Fatal("uuid")
	}
}
