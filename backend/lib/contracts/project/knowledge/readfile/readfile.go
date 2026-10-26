// Package readfile adalah kontrak bus project.knowledge.read-file (provider T05, consumer T08).
//
// Offset dan Limit dalam BYTE. Potongan selalu pada batas karakter UTF-8: Offset di tengah karakter maju ke
// karakter berikutnya; akhir potongan mundur ke batas karakter. Membaca lanjutan: Offset = NextOffset.
// Tidak ketemu -> Found=false, Similar berisi maks 5 path mirip, err nil.
// Error: tipe salah -> bus.ErrBadRequestType; ProjectID/Path tidak valid atau Offset di luar file ->
// errs.ErrInvalid dengan pesan yang memandu.
//
// Whole hanya untuk consumer yang tidak meneruskan isi mentah ke klien (T18 outline/section); read_file tidak memakainya.
package readfile

import "time"

const DefaultLimit = 65536 // Limit 0 -> DefaultLimit; maks Config.MCPMaxResponseBytes

type Request struct {
	ProjectID string
	Path      string // satu "/" di depan ditoleransi
	Offset    int
	Limit     int
	Whole     bool // true: kembalikan isi utuh (<= MAX_FILE_BYTES), abaikan Limit dan clamp MaxReadBytes; Offset WAJIB 0
}

type Response struct {
	Found      bool
	Path       string
	Content    string // potongan
	SizeBytes  int    // len(Content) dalam byte
	TotalBytes int    // ukuran file utuh
	NextOffset int    // offset byte setelah potongan
	Truncated  bool   // NextOffset < TotalBytes
	Similar    []string
	UpdatedAt  time.Time // UTC, diisi bila Found
}
