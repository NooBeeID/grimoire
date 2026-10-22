// Package upsertfiles: kontrak project.knowledge.upsert-files (T14).
// Provider TIDAK memeriksa keanggotaan: consumer WAJIB check-access lebih dulu.
// Provider selalu membatasi query pada Request.ProjectID (rule 6.10). Tidak menghapus file.
package upsertfiles

const SourceGitSync = "git_sync"

type Actor struct{ UserID, Role, Label string }
type Meta struct{ IP, UserAgent string }

type Item struct {
	Index    int    // urutan dari consumer; dipantulkan di ItemResult
	Path     string // relatif terhadap folder git (tanpa TargetPrefix)
	Content  []byte // nil bila TooLarge
	TooLarge bool
}

type Request struct {
	ProjectID    string
	Actor        Actor
	TargetPrefix string // "" = root; divalidasi provider (invalid -> *errs.Error CodeInvalid, sebelum tulis)
	Source       string // SourceGitSync
	Meta         Meta
	Items        []Item
}

type ItemResult struct {
	Index  int
	Path   string // path akhir di knowledge (setelah prefix + normalisasi), atau path mentah bila ditolak
	Status string // created|updated|unchanged|skipped|rejected
	Reason string // kode reason knowledge (mis. "too_large", "hidden"); "" bila sukses
}

type Summary struct{ Created, Updated, Unchanged, Skipped, Rejected int }

type Response struct {
	Items   []ItemResult // urut Index
	Summary Summary
}
