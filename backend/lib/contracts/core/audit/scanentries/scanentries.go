// Package scanentries adalah kontrak bus core.audit.scan-entries (provider T02 core/auditlog,
// feature scanentries; ditambah T11). Baca-saja, selalu dibatasi ProjectID (rules 6.10).
//
// Request dikirim sebagai nilai scanentries.Request; tipe lain -> error membungkus
// bus.ErrBadRequestType. Request invalid -> error membungkus errs.ErrInvalid (pesan tanpa nilai input).
// Paging keyset ASC: kirim Response.Next sebagai Request.After sampai Next == nil.
package scanentries

import "time"

const (
	DefaultLimit = 500
	MaxLimit     = 1000
	MaxActions   = 10
)

type Request struct {
	ProjectID string    // wajib, uuid
	Actions   []string  // wajib, 1..MaxActions, tiap item format aksi audit valid
	From      time.Time // wajib, inklusif
	To        time.Time // nol -> now; eksklusif; harus > From
	After     *Cursor   // nil = dari awal
	Limit     int       // 0 -> DefaultLimit; > MaxLimit -> invalid
}

type Cursor struct {
	OccurredAt time.Time
	ID         string // uuid
}

type Entry struct {
	ID         string
	OccurredAt time.Time
	ActorType  string
	ActorID    string // "" bila NULL
	Action     string
	Target     string // "" bila NULL
	Status     string
	Args       map[string]any // jsonb; tidak pernah nil (kosong -> map{})
	Result     map[string]any // jsonb; tidak pernah nil
}

type Response struct {
	Entries []Entry // tidak pernah nil
	Next    *Cursor // nil bila tidak ada halaman berikutnya
}
