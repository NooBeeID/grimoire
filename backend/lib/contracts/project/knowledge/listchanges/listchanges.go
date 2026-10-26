// Package listchanges adalah kontrak bus project.knowledge.list-changes (provider T18, consumer T18 list_recent_changes).
//
// Semua hasil dibatasi pada ProjectID (rule 6.10). Path tanpa awalan "/". Hasil diurut UpdatedAt menurun,
// lalu Path (urutan byte) menaik. File yang sudah dihapus tidak muncul (hard delete).
// Error: tipe request salah -> bus.ErrBadRequestType; ProjectID bukan uuid, Since nol, atau Prefix tidak valid ->
// error membungkus errs.ErrInvalid. Tanpa perubahan -> Items kosong (bukan nil), err nil.
package listchanges

import "time"

const (
	DefaultLimit = 20
	MaxLimit     = 50
)

type Request struct {
	ProjectID string
	Since     time.Time // wajib (nol -> errs.ErrInvalid); filter updated_at >= Since
	Prefix    string    // "" = semua; satu "/" di depan/belakang ditoleransi
	Limit     int       // 0 -> DefaultLimit; > MaxLimit -> MaxLimit
}

type Item struct {
	Path                 string
	SizeBytes            int
	CreatedAt, UpdatedAt time.Time // UTC
}

type Response struct {
	Items []Item // urut UpdatedAt DESC, Path byte order ASC; tidak pernah nil
	Total int    // semua baris cocok, bukan hanya halaman
}
