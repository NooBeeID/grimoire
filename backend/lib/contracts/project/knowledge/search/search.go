// Package search adalah kontrak bus project.knowledge.search (provider T05, consumer T08).
//
// Query dibersihkan provider (tanpa sintaks tsquery mentah); frasa dalam tanda kutip didukung.
// Query tanpa kata -> Items kosong, Total 0, err nil. Snippet memakai **term** sebagai sorotan.
// Error: tipe salah -> bus.ErrBadRequestType; ProjectID/Prefix tidak valid -> errs.ErrInvalid.
package search

const (
	DefaultLimit = 20
	MaxLimit     = 50
)

type Request struct {
	ProjectID string
	Query     string
	Prefix    string // opsional; satu "/" di depan ditoleransi
	Limit     int    // 0 -> DefaultLimit; > MaxLimit -> MaxLimit
}

type Item struct {
	Path    string
	Score   float64
	Snippet string
	Section string // heading bagian cuplikan (T20); "" di luar heading
}

type Response struct {
	Items []Item
	Total int // jumlah dokumen cocok (sebelum limit)
	// Expanded true bila sinonim atau koreksi typo dipakai (T20). Aditif; zero value = tidak diperluas.
	Expanded bool
}
