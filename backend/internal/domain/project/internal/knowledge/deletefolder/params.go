package deletefolder

// params adalah input mentah dari request (tanpa body).
type params struct {
	ProjectID string
	Path      string // r.URL.Query().Get("path"), mentah
}

// Output adalah respons 200.
type Output struct {
	Deleted int `json:"deleted"`
}
