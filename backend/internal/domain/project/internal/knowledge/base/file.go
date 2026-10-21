package base

import "time"

// File adalah satu file knowledge. Waktu selalu UTC (store memanggil .UTC()).
type File struct {
	ID, ProjectID, Path string
	Title, Content      string
	Hash                []byte
	SizeBytes           int
	CreatedAt           time.Time
	CreatedBy           string
	CreatedByLabel      string
	UpdatedAt           time.Time
	UpdatedBy           string
	UpdatedByLabel      string
}

// NewFile menyiapkan file baru dari path tervalidasi dan isi siap pakai. Waktu diisi DB (now()).
func NewFile(projectID, path, content string, actor Actor) File {
	return File{
		ProjectID: projectID, Path: path,
		Title: Title(content), Content: content, Hash: Hash(content), SizeBytes: len(content),
		CreatedBy: actor.ID, CreatedByLabel: actor.Label,
		UpdatedBy: actor.ID, UpdatedByLabel: actor.Label,
	}
}

// WithContent mengganti isi (Title, Hash, SizeBytes, Updated* dari actor). Created* tidak berubah.
func (f File) WithContent(content string, actor Actor) File {
	f.Title = Title(content)
	f.Content = content
	f.Hash = Hash(content)
	f.SizeBytes = len(content)
	f.UpdatedBy = actor.ID
	f.UpdatedByLabel = actor.Label
	return f
}

// Version adalah hex sha256 isi (D7).
func (f File) Version() string { return Version(f.Hash) }

// Meta adalah metadata file tanpa isi (untuk pohon dan daftar).
type Meta struct {
	Path      string
	SizeBytes int
	UpdatedAt time.Time
}

type UserRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type FileDTO struct {
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	SizeBytes int       `json:"size_bytes"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy UserRef   `json:"created_by"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy UserRef   `json:"updated_by"`
}

// ConflictCurrent adalah isi details.current pada 409 version_conflict.
type ConflictCurrent struct {
	Content        string    `json:"content"`
	Version        string    `json:"version"`
	UpdatedAt      time.Time `json:"updated_at"`
	UpdatedByLabel string    `json:"updated_by_label"`
}

func (f File) ToDTO() FileDTO {
	return FileDTO{
		Path: f.Path, Title: f.Title, Content: f.Content, SizeBytes: f.SizeBytes, Version: f.Version(),
		CreatedAt: f.CreatedAt, CreatedBy: UserRef{ID: f.CreatedBy, Label: f.CreatedByLabel},
		UpdatedAt: f.UpdatedAt, UpdatedBy: UserRef{ID: f.UpdatedBy, Label: f.UpdatedByLabel},
	}
}

func (f File) ToConflict() ConflictCurrent {
	return ConflictCurrent{
		Content: f.Content, Version: f.Version(), UpdatedAt: f.UpdatedAt, UpdatedByLabel: f.UpdatedByLabel,
	}
}
