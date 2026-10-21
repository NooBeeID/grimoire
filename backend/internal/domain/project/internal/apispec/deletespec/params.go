package deletespec

// Params diambil apa adanya dari URL; tidak divalidasi di handler.
// Nama tak valid cukup tidak ditemukan (404) agar tidak membuka oracle.
type Params struct {
	ProjectID string
	Name      string
}
