package base

import (
	"sort"
)

// Status laporan keseluruhan.
type Status string

const (
	StatusSuccess Status = "success"
	StatusPartial Status = "partial"
	StatusFailed  Status = "failed"
)

// Status per item.
const (
	ItemCreated   = "created"
	ItemUpdated   = "updated"
	ItemUnchanged = "unchanged"
	ItemSkipped   = "skipped"
	ItemRejected  = "rejected"
)

// MaxStoredReportItems membatasi item yang disimpan di git_sync_configs.last_result.
const MaxStoredReportItems = 500

// Issue adalah masalah validasi satu spec (hanya api_spec).
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Pointer string `json:"pointer"`
	Line    int    `json:"line"`
}

// ReportItem adalah hasil satu file. Field OpenAPIVersion/Title/EndpointCount hanya untuk api_spec.
type ReportItem struct {
	RepoPath       string  `json:"repo_path"`
	Target         string  `json:"target"`
	Status         string  `json:"status"`
	ReasonCode     string  `json:"reason_code"`
	Reason         string  `json:"reason"`
	Issues         []Issue `json:"issues"`
	OpenAPIVersion string  `json:"openapi_version,omitempty"`
	Title          string  `json:"title,omitempty"`
	EndpointCount  int     `json:"endpoint_count,omitempty"`
}

// Summary menghitung item per status.
type Summary struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
	Rejected  int `json:"rejected"`
}

// Report adalah laporan satu sync.
type Report struct {
	Status       Status
	Summary      Summary
	Items        []ReportItem
	Truncated    bool
	ErrorCode    string
	ErrorMessage string
}

// NewReport mengurutkan item per RepoPath dan menghitung Summary dari semua item. Status diturunkan.
func NewReport(items []ReportItem) Report {
	items = append([]ReportItem(nil), items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].RepoPath < items[j].RepoPath })
	r := Report{Items: items}
	r.Summary = summarize(items)
	r.DeriveStatus()
	return r
}

func summarize(items []ReportItem) Summary {
	var s Summary
	for _, it := range items {
		switch it.Status {
		case ItemCreated:
			s.Created++
		case ItemUpdated:
			s.Updated++
		case ItemUnchanged:
			s.Unchanged++
		case ItemSkipped:
			s.Skipped++
		case ItemRejected:
			s.Rejected++
		}
	}
	return s
}

// DeriveStatus: failed bila ErrorCode terisi; partial bila ada rejected; selain itu success.
func (r *Report) DeriveStatus() {
	switch {
	case r.ErrorCode != "":
		r.Status = StatusFailed
	case r.Summary.Rejected > 0:
		r.Status = StatusPartial
	default:
		r.Status = StatusSuccess
	}
}

// FailedReport adalah laporan gagal tanpa item.
func FailedReport(code, msg string) Report {
	r := Report{ErrorCode: code, ErrorMessage: msg, Items: []ReportItem{}}
	r.DeriveStatus()
	return r
}

// CapForStorage memotong Items ke n dan menandai Truncated. Summary tidak berubah
// (sudah dihitung dari semua item).
func (r Report) CapForStorage(n int) Report {
	if n < 0 {
		n = 0
	}
	if len(r.Items) > n {
		r.Items = append([]ReportItem(nil), r.Items[:n]...)
		r.Truncated = true
	}
	return r
}
