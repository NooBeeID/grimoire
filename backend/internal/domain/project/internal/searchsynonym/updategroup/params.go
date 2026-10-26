package updategroup

// Input adalah body PUT /projects/{projectID}/search/synonyms/{groupID}. Hanya bentuk; validasi ada di entity.
type Input struct {
	Terms []string `json:"terms"`
}
