package httpx

import (
	"net/http"

	"grimoire/internal/infra/http/respond"
	"grimoire/lib/common/errs"
)

// TraceHeader adalah header trace id pada request/respons.
const TraceHeader = respond.TraceHeader

// WriteError menulis error sebagai JSON standar (lihat respond.WriteError).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	respond.WriteError(w, r, err)
}

// StatusFor memetakan kode error ke status HTTP; kode tak dikenal menjadi 500.
func StatusFor(code errs.Code) int { return respond.StatusFor(code) }

// RegisterCodeStatus mendaftarkan status untuk kode baru; duplikat = panic.
func RegisterCodeStatus(code errs.Code, status int) { respond.RegisterCodeStatus(code, status) }
