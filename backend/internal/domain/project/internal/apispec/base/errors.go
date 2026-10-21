package base

import "grimoire/lib/common/errs"

const (
	ReasonNameTaken             = "NAME_TAKEN"
	ReasonNameWithMultipleFiles = "NAME_WITH_MULTIPLE_FILES"
	ReasonNoFiles               = "NO_FILES"
	ReasonNotMultipart          = "NOT_MULTIPART"
	// ReasonInvalidName (upload.go) juga dipakai untuk 400 renamespec.
)

// Sentinel dipakai bersama (pointer sama). Usecase TIDAK boleh memodifikasinya.
var (
	ErrProjectNotFound = errs.New(errs.CodeNotFound, "project tidak ditemukan")
	ErrSpecNotFound    = errs.New(errs.CodeNotFound, "spec tidak ditemukan")
	ErrInvalidName     = errs.WithDetails(errs.CodeInvalid,
		"nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter",
		map[string]any{"reason": ReasonInvalidName, "fields": map[string]string{"name": "nama spec tidak valid"}})
	ErrInvalidPath = errs.New(errs.CodeInvalid, "path tidak valid") // internal -> FileResult
	ErrNameTaken   = errs.WithDetails(errs.CodeConflict, "nama spec sudah dipakai di project ini",
		map[string]any{"reason": ReasonNameTaken})
	ErrNoFiles = errs.WithDetails(errs.CodeInvalid, "tidak ada file",
		map[string]any{"reason": ReasonNoFiles})
	ErrNameWithMultipleFiles = errs.WithDetails(errs.CodeInvalid,
		"field name hanya boleh dipakai saat mengunggah tepat satu file",
		map[string]any{"reason": ReasonNameWithMultipleFiles})
	ErrNotMultipart = errs.WithDetails(errs.CodeInvalid, "request harus multipart/form-data",
		map[string]any{"reason": ReasonNotMultipart})
	ErrPayloadTooLarge = errs.New(errs.CodePayloadTooLarge, "ukuran upload melebihi batas")
)
