package constants

// PasswordGateExempt berisi endpoint yang tetap boleh diakses selama
// must_change_password=true. Key: "METHOD path" persis.
var PasswordGateExempt = map[string]struct{}{
	"POST /api/auth/login":           {},
	"GET /api/auth/me":               {},
	"POST /api/auth/change-password": {},
	"POST /api/auth/logout":          {},
}
