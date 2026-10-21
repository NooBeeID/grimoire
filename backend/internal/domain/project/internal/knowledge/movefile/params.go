package movefile

// Request adalah body POST. Field tak dikenal ditolak respond.DecodeJSON (400).
type Request struct {
	From string `json:"from"`
	To   string `json:"to"`
}
