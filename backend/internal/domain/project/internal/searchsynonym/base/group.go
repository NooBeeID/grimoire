// Package base berisi entity dan business rule module project/searchsynonym (kamus sinonim pencarian).
// Semua fungsi di package ini murni: tanpa context dan tanpa I/O.
package base

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MinTerms            = 2
	MaxTerms            = 10
	MaxTermRunes        = 64
	MaxGroupsPerProject = 500
)

// Group adalah satu kelompok term setara. Waktu selalu UTC (store memanggil .UTC()).
type Group struct {
	ID, ProjectID  string
	Terms          []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CreatedBy      string
	CreatedByLabel string
	UpdatedBy      string
	UpdatedByLabel string
}

type GroupDTO struct {
	ID             string    `json:"id"`
	Terms          []string  `json:"terms"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedByLabel string    `json:"created_by_label"`
	UpdatedByLabel string    `json:"updated_by_label"`
}

// NormalizeTerms memvalidasi dan menormalkan term: trim, huruf kecil, spasi beruntun diringkas.
// Hasil valid -> (terms, ""); selain itu (nil, alasan). Karakter yang diizinkan: huruf, angka, spasi, - dan _.
func NormalizeTerms(raw []string) ([]string, Reason) {
	switch {
	case len(raw) < MinTerms:
		return nil, ReasonTooFewTerms
	case len(raw) > MaxTerms:
		return nil, ReasonTooManyTerms
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, t := range raw {
		t = strings.ToLower(strings.Join(strings.Fields(t), " "))
		switch {
		case t == "":
			return nil, ReasonTermEmpty
		case utf8.RuneCountInString(t) > MaxTermRunes:
			return nil, ReasonTermTooLong
		}
		for _, r := range t {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' && r != '_' {
				return nil, ReasonTermInvalidChar
			}
		}
		if seen[t] {
			return nil, ReasonDuplicateTerm
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, ""
}

// NewGroup menyiapkan kelompok baru dari term yang SUDAH dinormalkan. Waktu diisi DB (now()).
func NewGroup(projectID string, terms []string, a Actor) Group {
	return Group{
		ProjectID: projectID, Terms: terms,
		CreatedBy: a.ID, CreatedByLabel: a.Label,
		UpdatedBy: a.ID, UpdatedByLabel: a.Label,
	}
}

// WithTerms mengganti term (Updated* dari actor). Created* tidak berubah.
func (g Group) WithTerms(terms []string, a Actor) Group {
	g.Terms = terms
	g.UpdatedBy = a.ID
	g.UpdatedByLabel = a.Label
	return g
}

func (g Group) ToDTO() GroupDTO {
	terms := g.Terms
	if terms == nil {
		terms = []string{}
	}
	return GroupDTO{
		ID: g.ID, Terms: terms, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
		CreatedByLabel: g.CreatedByLabel, UpdatedByLabel: g.UpdatedByLabel,
	}
}
