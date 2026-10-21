// Package verify adalah kontrak bus project.apikey.verify (provider T04 project/apikey, consumer T07).
//
// Request dikirim sebagai nilai verify.Request (bukan pointer). Tipe lain -> error membungkus bus.ErrBadRequestType.
//
// Semantik:
//   - Key valid dan aktif           -> Found=true,  KeyID/KeyName/Prefix/ProjectID terisi, Reason "".
//   - Format salah                  -> Found=false, Reason=ReasonMalformed, Prefix "" (tanpa query DB).
//   - Prefix tidak dikenal          -> Found=false, Reason=ReasonUnknown,   Prefix terisi.
//   - Prefix dikenal, secret salah  -> Found=false, Reason=ReasonMismatch,  Prefix terisi.
//   - Key dicabut (secret benar)    -> Found=false, Reason=ReasonRevoked,   Prefix terisi.
//   - Error DB                      -> err != nil. Consumer WAJIB menolak (fail-closed).
//
// Reason dan Prefix HANYA untuk audit internal (mcp.auth.rejected). DILARANG meneruskan Reason ke agent/klien:
// respons ke luar harus seragam untuk semua Found=false.
// Tanpa cache: key yang dicabut langsung Found=false pada panggilan berikutnya.
// Provider tidak mengaudit dan tidak me-touch; itu tugas consumer (T07).
//
// Pola consumer (T07):
//
//	res, err := bus.CallTyped[verify.Response](ctx, b, constants.BUS_PROJECT_APIKEY_VERIFY, verify.Request{RawKey: raw})
//	if err != nil { /* tolak 401/503, log error; fail-closed */ }
//	if !res.Found { /* tolak seragam; audit mcp.auth.rejected dengan res.Prefix + res.Reason */ }
package verify

const (
	ReasonMalformed = "malformed"
	ReasonUnknown   = "unknown"
	ReasonRevoked   = "revoked"
	ReasonMismatch  = "mismatch"
)

type Request struct {
	RawKey string // "gr_<prefix>_<secret>"; spasi di tepi diabaikan provider
}

type Response struct {
	Found     bool
	KeyID     string
	KeyName   string
	Prefix    string
	ProjectID string
	Reason    string // kosong bila Found=true
}
