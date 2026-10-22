// Package base berisi entity dan business rule module core/gitcredential (PAT Git per user).
// Tanpa I/O; subpaket audit dan store memegang I/O bersama.
package base

import "strings"

// Provider adalah jenis penyedia Git yang didukung.
type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
)

// ParseProvider memvalidasi provider (case-insensitive, hasil lowercase).
func ParseProvider(raw string) (Provider, error) {
	switch p := Provider(strings.ToLower(strings.TrimSpace(raw))); p {
	case ProviderGitHub, ProviderGitLab:
		return p, nil
	}
	return "", ErrInvalidProvider
}
