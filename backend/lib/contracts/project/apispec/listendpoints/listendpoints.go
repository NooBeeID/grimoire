// Package listendpoints adalah kontrak bus project.apispec.list-endpoints (provider T06, consumer T09).
// Clamp (bukan error): Limit<=0 -> DefaultLimit, >MaxLimit -> MaxLimit; Offset<0 -> 0.
// Query di-trim, dipotong MaxQueryRunes; dicocokkan ILIKE (escape % _ \) pada path, summary, operation_id.
// Tag dicocokkan persis (case-sensitive) ke salah satu tag endpoint. Spec "" = semua spec project.
// Spec diisi tapi tidak ada -> SpecFound=false, Specs berisi nama spec valid, Items kosong, err nil.
// Urut: Spec, Path, method kanonik (GET,PUT,POST,DELETE,OPTIONS,HEAD,PATCH,TRACE).
// Truncated = Offset+len(Items) < Total; NextOffset = Offset+len(Items) bila Truncated, else 0.
// ProjectID bukan uuid -> error membungkus errs.ErrInvalid.
package listendpoints

const (
	DefaultLimit  = 100
	MaxLimit      = 300
	MaxQueryRunes = 200
)

type Request struct {
	ProjectID, Spec, Tag, Query string
	Limit, Offset               int
}

// SpecFound bernilai true bila Request.Spec == "".
type Response struct {
	SpecFound  bool
	Specs      []string // diisi bila !SpecFound
	Items      []Endpoint
	Total      int
	Truncated  bool
	NextOffset int
}

type Endpoint struct {
	Spec, Method, Path string
	OperationID        string // "" bila tidak ada
	Summary            string
	Tags               []string
	Deprecated         bool
}
