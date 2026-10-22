package base

import "time"

// ReportDTO adalah bentuk JSON laporan (juga format last_result di DB). Tidak punya field token.
type ReportDTO struct {
	Status       string       `json:"status"`
	Summary      Summary      `json:"summary"`
	Items        []ReportItem `json:"items"`
	Truncated    bool         `json:"truncated"`
	ErrorCode    string       `json:"error_code"`
	ErrorMessage string       `json:"error_message"`
}

// SyncedByDTO adalah pelaku sync terakhir (label snapshot).
type SyncedByDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ConfigDTO adalah bentuk JSON config sync.
type ConfigDTO struct {
	Provider     string       `json:"provider"`
	Host         string       `json:"host"`
	RepoURL      string       `json:"repo_url"`
	RepoPath     string       `json:"repo_path"`
	Branch       string       `json:"branch"`
	Folder       string       `json:"folder"`
	TargetPrefix string       `json:"target_prefix"`
	LastStatus   *string      `json:"last_status"`
	LastSyncedAt *time.Time   `json:"last_synced_at"`
	LastSyncedBy *SyncedByDTO `json:"last_synced_by"`
	LastResult   *ReportDTO   `json:"last_result"`
}

// ToDTO memetakan laporan; Items dan Issues tidak pernah null di JSON.
func (r Report) ToDTO() ReportDTO {
	items := make([]ReportItem, len(r.Items))
	for i, it := range r.Items {
		if it.Issues == nil {
			it.Issues = []Issue{}
		}
		items[i] = it
	}
	return ReportDTO{Status: string(r.Status), Summary: r.Summary, Items: items, Truncated: r.Truncated,
		ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage}
}

// ToReport memetakan DTO (dari last_result) kembali ke Report.
func (d ReportDTO) ToReport() Report {
	return Report{Status: Status(d.Status), Summary: d.Summary, Items: d.Items, Truncated: d.Truncated,
		ErrorCode: d.ErrorCode, ErrorMessage: d.ErrorMessage}
}
