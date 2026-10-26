package base

import "grimoire/lib/common/errs"

// Sentinel dipakai bersama; usecase DILARANG memodifikasinya.
var (
	ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
	ErrGroupNotFound   = errs.New(errs.CodeNotFound, "kelompok sinonim tidak ditemukan")
	ErrUnauthenticated = errs.New(errs.CodeUnauthorized, "perlu login")
)

// Invalid -> 400 INVALID_ARGUMENT, details {"reason": r}. Membungkus errs.ErrInvalid (dipakai consumer bus).
func Invalid(r Reason) *errs.Error {
	e := errs.WithDetails(errs.CodeInvalid, r.Message(), map[string]any{"reason": r})
	e.Err = errs.ErrInvalid
	return e
}

// TermInUse -> 409 CONFLICT, details {"reason": "term_in_use", "terms": [...]}.
func TermInUse(terms []string) *errs.Error {
	return errs.WithDetails(errs.CodeConflict, ReasonTermInUse.Message(),
		map[string]any{"reason": ReasonTermInUse, "terms": terms})
}

// GroupLimit -> 409 CONFLICT, details {"reason": "group_limit"}.
func GroupLimit() *errs.Error {
	return errs.WithDetails(errs.CodeConflict, ReasonGroupLimit.Message(),
		map[string]any{"reason": ReasonGroupLimit})
}
