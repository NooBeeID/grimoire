package downloadspec

// Params diambil apa adanya dari URL; tidak divalidasi di handler.
type Params struct {
	ProjectID string
	Name      string
}
