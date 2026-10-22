// Package upsertspecs: kontrak project.apispec.upsert-specs (T14).
// Provider TIDAK memeriksa keanggotaan; selalu membatasi query pada Request.ProjectID. Tidak menghapus spec.
package upsertspecs

const SourceGitSync = "git_sync"

type Actor struct{ UserID, Role, Label string }
type Meta struct{ IP, UserAgent string }

type Item struct {
	Index      int
	SourcePath string // path di repo, mis. "api/openapi/billing.yaml"
	Content    []byte
	TooLarge   bool
}

type Request struct {
	ProjectID string
	Actor     Actor
	Source    string
	Meta      Meta
	Items     []Item
}

type Issue struct {
	Code, Message, Pointer string
	Line                   int
}

type ItemResult struct {
	Index          int
	SourcePath     string
	Name           string // nama spec hasil DeriveName; "" bila tidak terbentuk
	Status         string // created|updated|unchanged|skipped|rejected
	ReasonCode     string // kode apispec (NOT_SPEC_FILE, INVALID_SPEC, DUPLICATE_NAME_IN_UPLOAD, ...)
	Reason         string // pesan Indonesia
	Issues         []Issue
	OpenAPIVersion string
	Title          string
	EndpointCount  int
}

type Summary struct{ Created, Updated, Unchanged, Skipped, Rejected int }

type Response struct {
	Items   []ItemResult
	Summary Summary
}
