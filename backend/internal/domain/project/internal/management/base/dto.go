package base

import (
	"sort"
	"strings"
	"time"
)

type ProjectDTO struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	CreatedBy      *string   `json:"created_by"`
	CreatedByLabel *string   `json:"created_by_label"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ContentCounts: nil berarti hitungan tidak tersedia (provider belum ada atau gagal).
type ContentCounts struct {
	Files *int `json:"files"`
	Specs *int `json:"specs"`
	Keys  *int `json:"keys"`
}

type ProjectSummary struct {
	ProjectDTO
	MyRole      *string       `json:"my_role"`
	AccessLevel Level         `json:"access_level"`
	IsMember    bool          `json:"is_member"`
	MemberCount int           `json:"member_count"`
	Counts      ContentCounts `json:"counts"`
}

type ProjectDetail struct {
	ProjectSummary
	OwnerCount int `json:"owner_count"`
}

type MemberDTO struct {
	UserID       string    `json:"user_id"`
	Name         *string   `json:"name"`
	Email        *string   `json:"email"`
	IsActive     bool      `json:"is_active"`
	Found        bool      `json:"found"`
	Role         string    `json:"role"`
	AddedAt      time.Time `json:"added_at"`
	AddedBy      *string   `json:"added_by"`
	AddedByLabel *string   `json:"added_by_label"`
}

type CandidateDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// NewSummary: myRole "" -> MyRole nil, IsMember false.
func NewSummary(p Project, level Level, myRole string, memberCount int, c ContentCounts) ProjectSummary {
	s := ProjectSummary{
		ProjectDTO: p.ToDTO(), AccessLevel: level, MemberCount: memberCount, Counts: c,
	}
	if myRole != "" {
		r := myRole
		s.MyRole = &r
		s.IsMember = true
	}
	return s
}

// SortMembers mengurutkan: owner dulu, lalu nama (case-insensitive, tanpa nama di akhir grup), lalu user_id.
func SortMembers(ms []MemberDTO) {
	sort.SliceStable(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if (a.Role == RoleOwner) != (b.Role == RoleOwner) {
			return a.Role == RoleOwner
		}
		if (a.Name == nil) != (b.Name == nil) {
			return a.Name != nil
		}
		if a.Name != nil {
			an, bn := strings.ToLower(*a.Name), strings.ToLower(*b.Name)
			if an != bn {
				return an < bn
			}
		}
		return a.UserID < b.UserID
	})
}
