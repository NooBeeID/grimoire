// Package runsync: POST /api/projects/{projectID}/git-sync/run. Menarik file dari repo Git
// (PAT milik user pemanggil) ke knowledge atau API spec lewat bus.
package runsync

import "grimoire/internal/domain/project/internal/gitsync/base"

// RunRequest adalah body POST /projects/{projectID}/git-sync/run.
type RunRequest struct {
	Kind         string `json:"kind"`
	URL          string `json:"url"`
	Branch       string `json:"branch"`
	Folder       string `json:"folder"`
	TargetPrefix string `json:"target_prefix"`
}

// RunResponse dibentuk dari base.ReportDTO dan base.ConfigDTO.
type RunResponse struct {
	Report base.ReportDTO `json:"report"`
	Config base.ConfigDTO `json:"config"`
}
