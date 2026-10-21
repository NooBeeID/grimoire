// Package base berisi entity dan rule bisnis module project/api-spec. Murni: tanpa I/O dan tanpa context.
package base

import (
	"crypto/sha256"
	"sort"
	"time"
)

const (
	FormatYAML = "yaml"
	FormatJSON = "json"
)

// DocMeta adalah metadata dokumen yang diisi dari hasil parse (openapi.Document).
type DocMeta struct{ Title, APIVersion, OpenAPIVersion string }

// Spec adalah satu spesifikasi OpenAPI tersimpan.
type Spec struct {
	ID, ProjectID, Name               string
	SourcePath                        *string
	Title, APIVersion, OpenAPIVersion string
	Content                           string // teks asli (tanpa BOM)
	Format                            string // FormatYAML | FormatJSON
	Hash                              [32]byte
	SizeBytes                         int
	CreatedAt                         time.Time
	CreatedBy                         string
	CreatedByLabel                    string
	UpdatedAt                         time.Time
	UpdatedBy                         string
	UpdatedByLabel                    string
}

// SpecSummary adalah Spec tanpa Content/Hash, ditambah jumlah endpoint.
type SpecSummary struct {
	ID, ProjectID, Name                       string
	SourcePath                                *string
	Title, APIVersion, OpenAPIVersion, Format string
	SizeBytes                                 int
	EndpointCount                             int
	CreatedAt                                 time.Time
	CreatedByLabel                            string
	UpdatedAt                                 time.Time
	UpdatedByLabel                            string
}

// Endpoint adalah satu baris indeks endpoint.
type Endpoint struct {
	Method      string // uppercase, salah satu Methods
	Path        string
	OperationID *string
	Summary     string   // dipotong 300 rune
	Tags        []string // dedup, urutan asli; tidak pernah nil
	Deprecated  bool
}

// Methods adalah urutan method kanonik.
var Methods = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE"}

// ValidMethod: m sudah uppercase.
func ValidMethod(m string) bool {
	for _, x := range Methods {
		if x == m {
			return true
		}
	}
	return false
}

// MethodOrder mengembalikan indeks di Methods; tidak dikenal -> len(Methods).
func MethodOrder(m string) int {
	for i, x := range Methods {
		if x == m {
			return i
		}
	}
	return len(Methods)
}

// SortEndpoints mengurutkan berdasarkan path, lalu MethodOrder.
func SortEndpoints(es []Endpoint) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].Path != es[j].Path {
			return es[i].Path < es[j].Path
		}
		return MethodOrder(es[i].Method) < MethodOrder(es[j].Method)
	})
}

// HashContent menghitung SHA-256 dari content.
func HashContent(content string) [32]byte { return sha256.Sum256([]byte(content)) }

// NewSpec membangun Spec baru dari hasil parse. Waktu di-UTC; Hash/SizeBytes dihitung dari content.
func NewSpec(projectID, name string, sourcePath *string, meta DocMeta, content, format string, actor Actor, now time.Time) Spec {
	now = now.UTC()
	return Spec{
		ProjectID: projectID, Name: name, SourcePath: sourcePath,
		Title: meta.Title, APIVersion: meta.APIVersion, OpenAPIVersion: meta.OpenAPIVersion,
		Content: content, Format: format, Hash: HashContent(content), SizeBytes: len(content),
		CreatedAt: now, CreatedBy: actor.ID, CreatedByLabel: actor.Label,
		UpdatedAt: now, UpdatedBy: actor.ID, UpdatedByLabel: actor.Label,
	}
}

// ApplyContent menimpa isi (saat updated). CreatedAt/By tetap; Updated* diisi actor/now.
func (s *Spec) ApplyContent(sourcePath *string, meta DocMeta, content, format string, actor Actor, now time.Time) {
	s.SourcePath = sourcePath
	s.Title, s.APIVersion, s.OpenAPIVersion = meta.Title, meta.APIVersion, meta.OpenAPIVersion
	s.Content, s.Format, s.Hash, s.SizeBytes = content, format, HashContent(content), len(content)
	s.UpdatedAt, s.UpdatedBy, s.UpdatedByLabel = now.UTC(), actor.ID, actor.Label
}

// Rename mengganti nama + Updated*; false bila nama sama (no-op).
func (s *Spec) Rename(newName string, actor Actor, now time.Time) bool {
	if s.Name == newName {
		return false
	}
	s.Name = newName
	s.UpdatedAt, s.UpdatedBy, s.UpdatedByLabel = now.UTC(), actor.ID, actor.Label
	return true
}

// MediaType mengembalikan nilai Content-Type unduhan raw untuk format spec.
func (s Spec) MediaType() string {
	switch s.Format {
	case FormatYAML:
		return "application/yaml; charset=utf-8"
	case FormatJSON:
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// FileName mengembalikan "<name>.<yaml|json>"; format tidak dikenal -> ".txt".
func (s Spec) FileName() string {
	switch s.Format {
	case FormatYAML, FormatJSON:
		return s.Name + "." + s.Format
	default:
		return s.Name + ".txt"
	}
}
