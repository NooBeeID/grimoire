// Package store berisi helper sqlx bersama module gitcredential: validasi uuid dan
// pemetaan pelanggaran constraint Postgres.
package store

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ConstraintProviderCheck = "git_credentials_provider_check"
	ConstraintHostCheck     = "git_credentials_host_check"
	ConstraintGitHubHost    = "git_credentials_github_host"
	ConstraintUserFK        = "git_credentials_user_id_fkey"
	ConstraintUnique        = "git_credentials_user_provider_host_key"

	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
)

// ValidUUID true bila semua id adalah uuid valid.
func ValidUUID(ids ...string) bool {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
	}
	return true
}

// Violation mengekstrak kode SQLSTATE dan nama constraint dari error Postgres.
// ok=false bila bukan error constraint (23503/23505/23514).
func Violation(err error) (code, constraint string, ok bool) {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return "", "", false
	}
	switch pg.Code {
	case codeUniqueViolation, codeForeignKeyViolation, codeCheckViolation:
		return pg.Code, pg.ConstraintName, true
	}
	return "", "", false
}
