// Package httpx menyediakan router chi, helper respons JSON seragam, dan seluruh
// middleware HTTP (recover, trace id, access log, security headers, CSRF, session via
// bus, rate limit, role guard, body limit, timeout).
package respond

import (
	"fmt"
	"net/http"
	"sync"

	"grimoire/lib/common/errs"
)

var (
	statusMu   sync.RWMutex
	codeStatus = map[errs.Code]int{
		errs.CodeNotFound:         http.StatusNotFound,
		errs.CodeForbidden:        http.StatusForbidden,
		errs.CodeConflict:         http.StatusConflict,
		errs.CodeInvalid:          http.StatusBadRequest,
		errs.CodeUnauthorized:     http.StatusUnauthorized,
		errs.CodeCSRFRequired:     http.StatusForbidden,
		errs.CodeRateLimited:      http.StatusTooManyRequests,
		errs.CodePayloadTooLarge:  http.StatusRequestEntityTooLarge,
		errs.CodeMethodNotAllowed: http.StatusMethodNotAllowed,
		errs.CodeTimeout:          http.StatusGatewayTimeout,
		errs.CodeUnavailable:      http.StatusServiceUnavailable,
		errs.CodeInternal:         http.StatusInternalServerError,
	}
)

// StatusFor memetakan kode error ke status HTTP (kode tak dikenal -> 500).
func StatusFor(code errs.Code) int {
	statusMu.RLock()
	defer statusMu.RUnlock()
	if s, ok := codeStatus[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// RegisterCodeStatus mendaftarkan kode error baru milik module. Panic bila kode sudah
// ada (termasuk bawaan) atau status di luar 400..599.
func RegisterCodeStatus(code errs.Code, status int) {
	if status < 400 || status > 599 {
		panic(fmt.Sprintf("httpx: status %d untuk kode %q di luar 400..599", status, code))
	}
	statusMu.Lock()
	defer statusMu.Unlock()
	if _, ok := codeStatus[code]; ok {
		panic(fmt.Sprintf("httpx: kode %q sudah terdaftar", code))
	}
	codeStatus[code] = status
}
