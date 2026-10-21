// Package listfiles adalah kontrak bus project.knowledge.list-files (provider T05, consumer T08).
//
// Semua hasil dibatasi pada ProjectID (rule 6.10). Path tanpa awalan "/".
// Error: tipe request salah -> bus.ErrBadRequestType; ProjectID bukan uuid atau Prefix tidak valid ->
// error membungkus errs.ErrInvalid dengan pesan yang memandu. Prefix tanpa file -> Items kosong, err nil.
package listfiles

import "time"

const (
	DefaultLimit = 100
	MaxLimit     = 500
)

type Request struct {
	ProjectID string
	Prefix    string // "" = root; satu "/" di depan/belakang ditoleransi
	Recursive bool   // true: semua file di bawah Prefix; false: hanya file langsung + Folders
	Limit     int    // 0 -> DefaultLimit; > MaxLimit -> MaxLimit
	Offset    int    // >= 0 (negatif -> errs.ErrInvalid)
}

type Item struct {
	Path      string
	SizeBytes int
	UpdatedAt time.Time
}

type Response struct {
	Items     []Item
	Folders   []string // path subfolder langsung bila !Recursive (urut), nil bila Recursive
	Total     int      // jumlah file yang cocok (bukan hanya halaman ini)
	Truncated bool     // Offset + len(Items) < Total
}
