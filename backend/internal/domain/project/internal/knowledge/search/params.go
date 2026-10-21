package search

// Input adalah parameter mentah dari query string.
type Input struct {
	ProjectID string
	Query     string // q mentah
	Prefix    string // prefix mentah
	Limit     string // limit mentah ("" = default)
}
