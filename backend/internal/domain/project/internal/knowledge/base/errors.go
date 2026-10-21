package base

import (
	"fmt"

	"grimoire/lib/common/errs"
)

// Sentinel dipakai bersama; usecase DILARANG memodifikasinya.
var (
	ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
	ErrFileNotFound    = errs.New(errs.CodeNotFound, "file tidak ditemukan")
	ErrFolderNotFound  = errs.New(errs.CodeNotFound, "folder tidak ditemukan atau kosong")
	ErrUnauthenticated = errs.New(errs.CodeUnauthorized, "perlu login")
	ErrPathExists      = errs.WithDetails(errs.CodeConflict, ReasonPathExists.Message(),
		map[string]any{"reason": ReasonPathExists})
)

// Invalid -> 400 INVALID_ARGUMENT, details {"reason": r}. Membungkus errs.ErrInvalid (dipakai bus consumer T08).
func Invalid(r Reason) *errs.Error { return InvalidMsg(r, r.Message()) }

// InvalidMsg sama dengan Invalid tetapi dengan pesan khusus (bus menyertakan path yang ditolak).
func InvalidMsg(r Reason, msg string) *errs.Error {
	e := errs.WithDetails(errs.CodeInvalid, msg, map[string]any{"reason": r})
	e.Err = errs.ErrInvalid
	return e
}

// TooLarge -> 413 PAYLOAD_TOO_LARGE, details {"reason": r, "max_bytes": max}.
func TooLarge(r Reason, max int64) *errs.Error {
	msg := r.Message()
	if r == ReasonTooLarge {
		msg = fmt.Sprintf("ukuran file melebihi batas %d byte", max)
	}
	return errs.WithDetails(errs.CodePayloadTooLarge, msg,
		map[string]any{"reason": r, "max_bytes": max})
}

// VersionConflict -> 409 CONFLICT, details {"reason": "version_conflict", "current": cur}.
func VersionConflict(cur ConflictCurrent) *errs.Error {
	return errs.WithDetails(errs.CodeConflict, ReasonVersionConflict.Message(),
		map[string]any{"reason": ReasonVersionConflict, "current": cur})
}
