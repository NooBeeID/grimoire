// Package base berisi entity dan business rule module project/knowledge.
// Semua fungsi di package ini murni: tanpa context dan tanpa I/O.
package base

// Reason adalah kode alasan stabil yang dikirim sebagai details.reason. "" = valid.
type Reason string

const (
	ReasonPathEmpty          Reason = "path_empty"
	ReasonPathAbsolute       Reason = "path_absolute"
	ReasonPathTraversal      Reason = "path_traversal"     // segmen "." atau ".."
	ReasonPathEmptySegment   Reason = "path_empty_segment" // "a//b", "a/" (untuk file)
	ReasonPathControlChar    Reason = "path_control_char"  // NUL, unicode.IsControl
	ReasonPathTooLong        Reason = "path_too_long"      // > 512 rune
	ReasonSegmentTooLong     Reason = "segment_too_long"   // > 255 rune
	ReasonHidden             Reason = "hidden"
	ReasonUnsupportedType    Reason = "unsupported_type"
	ReasonTooLarge           Reason = "too_large"
	ReasonInvalidEncoding    Reason = "invalid_encoding" // isi atau path bukan UTF-8 valid, atau isi mengandung NUL
	ReasonDuplicateInRequest Reason = "duplicate_in_request"
	ReasonTooManyFiles       Reason = "too_many_files"
	ReasonProjectFileLimit   Reason = "project_file_limit"
	ReasonInvalidOffset      Reason = "invalid_offset"
	ReasonInvalidVersion     Reason = "invalid_version" // bukan 64 hex huruf kecil
	ReasonSamePath           Reason = "same_path"       // move from == to
	ReasonPrefixRequired     Reason = "prefix_required" // deletefolder root
	ReasonInvalidLimit       Reason = "invalid_limit"   // limit bukan bilangan bulat (HTTP)
	ReasonNoFiles            Reason = "no_files"        // upload tanpa part file
	ReasonUploadTooLarge     Reason = "upload_too_large"
	ReasonPathExists         Reason = "path_exists"        // 409
	ReasonVersionConflict    Reason = "version_conflict"   // 409
	ReasonInvalidProjectID   Reason = "invalid_project_id" // bus
	ReasonNotMultipart       Reason = "not_multipart"
)

var reasonMessages = map[Reason]string{
	ReasonPathEmpty:          "path wajib diisi, mis. docs/auth/login.md",
	ReasonPathAbsolute:       "path harus relatif (tanpa awalan /), mis. docs/auth/login.md",
	ReasonPathTraversal:      "path tidak boleh memuat segmen . atau ..",
	ReasonPathEmptySegment:   "path tidak boleh memuat segmen kosong (// atau / di akhir)",
	ReasonPathControlChar:    "path tidak boleh memuat karakter kontrol",
	ReasonPathTooLong:        "path maksimal 512 karakter",
	ReasonSegmentTooLong:     "setiap bagian path maksimal 255 karakter",
	ReasonHidden:             "path yang diawali titik (mis. .git/) tidak didukung",
	ReasonUnsupportedType:    "hanya file .md, .markdown, dan .txt yang didukung",
	ReasonTooLarge:           "ukuran file melebihi batas",
	ReasonInvalidEncoding:    "isi harus teks UTF-8 tanpa karakter NUL",
	ReasonDuplicateInRequest: "path yang sama muncul lebih dari sekali dalam satu upload",
	ReasonTooManyFiles:       "jumlah file per upload melebihi batas",
	ReasonProjectFileLimit:   "project sudah mencapai batas jumlah file",
	ReasonInvalidOffset:      "offset di luar ukuran file",
	ReasonInvalidVersion:     "version wajib diisi dengan nilai dari getfile",
	ReasonSamePath:           "path tujuan sama dengan path asal",
	ReasonPrefixRequired:     "path folder wajib diisi; root tidak bisa dihapus sekaligus",
	ReasonInvalidLimit:       "limit harus bilangan bulat",
	ReasonNoFiles:            "upload tidak berisi file",
	ReasonUploadTooLarge:     "ukuran total upload melebihi batas",
	ReasonPathExists:         "file dengan path ini sudah ada",
	ReasonVersionConflict:    "file sudah diubah orang lain; muat ulang sebelum menyimpan",
	ReasonInvalidProjectID:   "ProjectID tidak valid",
	ReasonNotMultipart:       "body harus multipart/form-data dengan field files",
}

// Message mengembalikan pesan Indonesia yang memandu (dipakai HTTP dan bus).
// Untuk too_large pesan tanpa angka; usecase memformat angka batas sendiri.
func (r Reason) Message() string {
	if m, ok := reasonMessages[r]; ok {
		return m
	}
	return string(r)
}
