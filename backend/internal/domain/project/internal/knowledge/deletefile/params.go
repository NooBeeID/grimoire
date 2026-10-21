package deletefile

// params adalah input mentah dari request (tanpa body).
type params struct {
	ProjectID string
	Path      string // r.URL.Query().Get("path"), mentah
}
