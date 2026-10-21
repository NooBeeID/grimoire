package base

import "fmt"

// FileStatus adalah status hasil per file upload/validasi.
type FileStatus string

const (
	StatusCreated   FileStatus = "created"
	StatusUpdated   FileStatus = "updated"
	StatusUnchanged FileStatus = "unchanged"
	StatusSkipped   FileStatus = "skipped"
	StatusRejected  FileStatus = "rejected"
	StatusValid     FileStatus = "valid" // hanya validatespec
)

// reason_code per file (stabil, untuk i18n FE).
const (
	ReasonNotSpecFile           = "NOT_SPEC_FILE"
	ReasonFileTooLarge          = "FILE_TOO_LARGE"
	ReasonTooManyFiles          = "TOO_MANY_FILES"
	ReasonInvalidPath           = "INVALID_PATH"
	ReasonInvalidName           = "INVALID_NAME"
	ReasonDuplicateNameInUpload = "DUPLICATE_NAME_IN_UPLOAD"
	ReasonInvalidSpec           = "INVALID_SPEC"
	ReasonSpecLimitReached      = "SPEC_LIMIT_REACHED"
)

// Reasons memetakan reason_code ke pesan Indonesia (format fmt untuk yang berargumen).
var Reasons = map[string]string{
	ReasonNotSpecFile:           "bukan file spec (.yaml/.yml/.json)",
	ReasonFileTooLarge:          "ukuran file melebihi batas %d byte",
	ReasonTooManyFiles:          "melebihi batas jumlah file",
	ReasonInvalidPath:           "path tidak valid",
	ReasonInvalidName:           "nama spec tidak valid: huruf kecil, angka, '-', '_', '.'; 1-80 karakter",
	ReasonDuplicateNameInUpload: "nama duplikat dalam upload",
	ReasonInvalidSpec:           "dokumen OpenAPI tidak valid",
	ReasonSpecLimitReached:      "batas %d spec per project tercapai",
}

// ReasonMessage mengembalikan pesan untuk code; code tidak dikenal -> code itu sendiri.
func ReasonMessage(code string, args ...any) string {
	f, ok := Reasons[code]
	if !ok {
		return code
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}

// UploadDecision adalah keputusan per file saat upload.
type UploadDecision int

const (
	DecisionCreate UploadDecision = iota + 1
	DecisionUpdate
	DecisionUnchanged
	DecisionRejectLimit
)

// DecideUpload (berurutan): hash sama -> Unchanged; existing -> Update (tidak terkena batas);
// specCount >= maxSpecs -> RejectLimit; selain itu Create.
func DecideUpload(existing *Spec, newHash [32]byte, specCount, maxSpecs int) UploadDecision {
	switch {
	case existing != nil && existing.Hash == newHash:
		return DecisionUnchanged
	case existing != nil:
		return DecisionUpdate
	case specCount >= maxSpecs:
		return DecisionRejectLimit
	default:
		return DecisionCreate
	}
}

// UploadLimits adalah batas yang dipakai usecase upload.
type UploadLimits struct {
	MaxFileBytes int64
	MaxFiles     int
	MaxSpecs     int
}

// Issue adalah salinan netral dari openapi.Issue untuk DTO.
type Issue struct {
	Code    string
	Message string
	Pointer string // "" bila tidak ada
	Line    int    // 0 bila tidak diketahui
}

// FileResult adalah hasil per file.
type FileResult struct {
	File           string  // basename dari Content-Disposition
	SourcePath     *string // path ternormalisasi; nil bila tidak valid
	Name           string  // "" bila belum bisa ditentukan
	Status         FileStatus
	ReasonCode     string
	Reason         string
	Issues         []Issue // tidak pernah nil
	OpenAPIVersion string
	Title          string
	APIVersion     string
	EndpointCount  int
}

// UploadSummary menghitung hasil per status.
type UploadSummary struct{ Created, Updated, Unchanged, Skipped, Rejected int }

// Summarize menghitung ringkasan; status valid dan lainnya tidak dihitung di sini.
func Summarize(rs []FileResult) UploadSummary {
	var s UploadSummary
	for _, r := range rs {
		switch r.Status {
		case StatusCreated:
			s.Created++
		case StatusUpdated:
			s.Updated++
		case StatusUnchanged:
			s.Unchanged++
		case StatusSkipped:
			s.Skipped++
		case StatusRejected:
			s.Rejected++
		}
	}
	return s
}
