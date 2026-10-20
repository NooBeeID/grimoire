// Package record adalah kontrak bus core.audit.record (provider T02 core/auditlog).
//
// Request dikirim sebagai nilai record.Request (bukan pointer); tipe lain
// ditolak provider dengan error yang membungkus bus.ErrBadRequestType.
//
// Pola consumer (FAIL-OPEN, diputuskan T02, AC8):
//
//	_, err := bus.CallTyped[record.Response](ctx, d.Bus, constants.BUS_CORE_AUDIT_RECORD, req)
//	if err != nil {
//		log.ErrorContext(ctx, "audit write failed", "action", req.Action, "err", err)
//	}
//	// lanjutkan melayani permintaan; jangan kembalikan error ke klien/agent.
//
// Untuk Publish: if err := d.Bus.Publish(ctx, constants.BUS_CORE_AUDIT_RECORD, req); err != nil { log sama }.
// Jangan pernah me-log req.Args/req.Result. Jangan kirim isi file: kirim metadata (path, bytes, count).
package record

import "time"

// Request adalah satu entri audit yang akan ditulis.
type Request struct {
	ID           string         // opsional; kosong -> diisi server (uuid v7)
	OccurredAt   time.Time      // opsional; nol -> waktu server
	ActorType    string         // ActorUser | ActorAPIKey | ActorAnonymous | ActorSystem
	ActorID      string         // uuid; wajib untuk user/api_key, wajib kosong untuk anonymous/system
	ActorLabel   string         // snapshot: email/nama user, atau "<nama key> (<prefix>)"; anonymous: prefix key bila ada
	ProjectID    string         // uuid atau kosong
	ProjectLabel string         // snapshot nama project
	Action       string         // lihat actions.go
	TargetType   string         // mis. "file", "api_spec", "user", "project", "api_key"
	Target       string         // mis. "docs/auth/login.md"
	Args         map[string]any // akan disanitasi server
	Result       map[string]any // ringkasan: bytes, count, error_code; disanitasi server
	Status       string         // StatusOK | StatusError | StatusDenied
	IP           string         // "1.2.3.4", "::1", atau "host:port"; invalid -> disimpan NULL
	UserAgent    string
	TraceID      string // kosong -> reqctx.TraceID(ctx)
}

// Response menyatakan apakah entri tertulis.
type Response struct{ Recorded bool }

const (
	ActorUser      = "user"
	ActorAPIKey    = "api_key"
	ActorAnonymous = "anonymous"
	ActorSystem    = "system"

	StatusOK     = "ok"
	StatusError  = "error"
	StatusDenied = "denied"
)
