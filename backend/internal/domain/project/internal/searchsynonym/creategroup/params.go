package creategroup

// Input adalah body POST /projects/{projectID}/search/synonyms. Hanya bentuk; validasi ada di entity.
type Input struct {
	Terms []string `json:"terms"`
}
