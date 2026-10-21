package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	CodeUniqueViolation = "23505"
	CodeFKViolation     = "23503"
)

// Violation mengembalikan SQLSTATE dan nama constraint bila err (atau error yang dibungkusnya) adalah *pgconn.PgError.
func Violation(err error) (code, constraint string, ok bool) {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code, pe.ConstraintName, true
	}
	return "", "", false
}
