// Package errs berisi error sentinel dan tipe Error bersama.
package errs

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrInvalid      = errors.New("invalid argument")
	ErrUnauthorized = errors.New("unauthorized")
)

// Code adalah kode error yang stabil untuk klien.
type Code string

const (
	CodeNotFound         Code = "NOT_FOUND"          // 404
	CodeForbidden        Code = "FORBIDDEN"          // 403
	CodeConflict         Code = "CONFLICT"           // 409
	CodeInvalid          Code = "INVALID_ARGUMENT"   // 400
	CodeUnauthorized     Code = "UNAUTHORIZED"       // 401
	CodeCSRFRequired     Code = "CSRF_REQUIRED"      // 403
	CodeRateLimited      Code = "RATE_LIMITED"       // 429
	CodePayloadTooLarge  Code = "PAYLOAD_TOO_LARGE"  // 413
	CodeMethodNotAllowed Code = "METHOD_NOT_ALLOWED" // 405
	CodeTimeout          Code = "TIMEOUT"            // 504
	CodeUnavailable      Code = "UNAVAILABLE"        // 503
	CodeInternal         Code = "INTERNAL"           // 500
)

// Error adalah error terstruktur. Err tidak pernah dikirim ke klien.
type Error struct {
	Code    Code
	Message string // aman ditampilkan ke klien
	Details any    // opsional, aman ditampilkan
	Err     error  // penyebab internal
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func (e *Error) Unwrap() error { return e.Err }

// New membuat Error tanpa penyebab.
func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

// Wrap membuat Error dengan penyebab internal.
func Wrap(code Code, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}

// WithDetails membuat Error dengan detail yang aman ditampilkan.
func WithDetails(code Code, msg string, details any) *Error {
	return &Error{Code: code, Message: msg, Details: details}
}
