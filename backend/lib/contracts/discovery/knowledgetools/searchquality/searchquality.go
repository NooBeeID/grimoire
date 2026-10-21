// Package searchquality adalah kontrak bus discovery.knowledgetools.search-quality-report
// (provider T11 knowledgetools/searchquality, consumer cmd/cli). Baca-saja.
// Request invalid -> error membungkus errs.ErrInvalid. Tipe salah -> bus.ErrBadRequestType.
package searchquality

import "time"

type Request struct {
	ProjectID string        // wajib, uuid
	Since     time.Time     // wajib, inklusif
	Until     time.Time     // nol -> now; eksklusif; harus > Since
	Window    time.Duration // 0 -> 120s; 1s..1h
}

type Response struct {
	Report Report `json:"report"`
}

type Report struct {
	ProjectID      string       `json:"project_id"`
	Since          time.Time    `json:"since"`
	Until          time.Time    `json:"until"`
	WindowSeconds  int          `json:"window_seconds"`
	Searches       int          `json:"searches"`        // search status ok dengan At di [Since, Until)
	ZeroResult     Metric       `json:"zero_result"`     // denominator = search dengan total terbaca
	FollowUp       Metric       `json:"follow_up"`       // denominator = search dengan result_paths
	Reformulation  Metric       `json:"reformulation"`   // denominator = Searches
	RankOpened     []RankBucket `json:"rank_opened"`     // "1".."10","not_in_results", urutan tetap, selalu 11 bucket
	LegacySearches int          `json:"legacy_searches"` // search ok tanpa result_paths (entri sebelum T11)
}

type Metric struct {
	Count       int     `json:"count"`
	Denominator int     `json:"denominator"`
	Rate        float64 `json:"rate"` // Count/Denominator, 0 bila Denominator 0
}

type RankBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

const NotInResults = "not_in_results"
