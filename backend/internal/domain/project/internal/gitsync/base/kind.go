// Package base berisi entity dan business rule module project/gitsync (sync knowledge dan API spec
// dari repo Git). Tanpa I/O; subpaket access, audit, dan store memegang I/O bersama.
package base

import (
	"path"
	"strings"
)

// Kind adalah jenis konten yang disinkronkan.
type Kind string

const (
	KindKnowledge Kind = "knowledge"
	KindAPISpec   Kind = "api_spec"
)

// ParseKind memvalidasi kind (tepat "knowledge" atau "api_spec").
func ParseKind(raw string) (Kind, error) {
	switch k := Kind(strings.TrimSpace(raw)); k {
	case KindKnowledge, KindAPISpec:
		return k, nil
	}
	return "", ErrInvalidKind
}

// AllowedExt true bila ekstensi path didukung kind ini (case-insensitive).
func (k Kind) AllowedExt(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	switch k {
	case KindKnowledge:
		return ext == ".md" || ext == ".txt"
	case KindAPISpec:
		return ext == ".yaml" || ext == ".yml" || ext == ".json"
	}
	return false
}

// SkipReason adalah kode reason item yang dilewati karena ekstensi tidak didukung,
// memakai kosakata module tujuan agar FE memakai i18n yang sama dengan upload (D14).
func (k Kind) SkipReason() string {
	if k == KindAPISpec {
		return "NOT_SPEC_FILE"
	}
	return "unsupported_type"
}
