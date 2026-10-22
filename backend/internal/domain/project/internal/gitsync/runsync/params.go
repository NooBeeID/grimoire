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

// Nama event SSE.
const (
	EventStart    = "start"
	EventProgress = "progress"
	EventWaiting  = "waiting"
	EventSaving   = "saving"
	EventDone     = "done"
	EventError    = "error"
)

// Sink menerima progress sync. Implementasi harus aman dipakai bersamaan dan tidak boleh memblok:
// Run.Execute memanggilnya dari beberapa goroutine.
type Sink interface {
	// Start dipanggil sekali setelah daftar file dan batas lolos; total = jumlah kandidat.
	Start(total int)
	// Progress dipanggil per kandidat yang selesai diproses (done monoton).
	Progress(done, total int, path string)
	// Waiting dipanggil saat sync menunggu rate limit provider.
	Waiting(retryAfterSeconds int, reason string)
	// Saving dipanggil sebelum fase tulis ke knowledge/api spec.
	Saving()
}

// NopSink membuang semua event.
type NopSink struct{}

func (NopSink) Start(int)                 {}
func (NopSink) Progress(int, int, string) {}
func (NopSink) Waiting(int, string)       {}
func (NopSink) Saving()                   {}

// StartEvent adalah payload event start.
type StartEvent struct {
	Total int `json:"total"`
}

// ProgressEvent adalah payload event progress.
type ProgressEvent struct {
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Path  string `json:"path"`
}

// WaitingEvent adalah payload event waiting.
type WaitingEvent struct {
	RetryAfterSeconds int    `json:"retry_after_seconds"`
	Reason            string `json:"reason"`
}

// SavingEvent adalah payload event saving.
type SavingEvent struct{}

// ErrorEvent adalah payload event error.
type ErrorEvent struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}
