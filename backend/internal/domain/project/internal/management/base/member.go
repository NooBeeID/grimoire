package base

import (
	"time"

	"grimoire/lib/contracts/core/auth/getuser"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Member struct {
	ProjectID    string
	UserID       string
	Role         string
	AddedBy      *string
	AddedByLabel *string
	AddedAt      time.Time
}

func ValidMemberRole(r string) bool { return r == RoleOwner || r == RoleMember }

// NewMember: role "" -> member; role tidak valid -> InvalidFields.
func NewMember(projectID, userID, role string, actor Actor, now time.Time) (Member, error) {
	if role == "" {
		role = RoleMember
	}
	if !ValidMemberRole(role) {
		return Member{}, InvalidFields(FieldErrors{"role": "role harus owner atau member"})
	}
	by, label := actor.ID, actor.Label
	return Member{
		ProjectID: projectID, UserID: userID, Role: role,
		AddedBy: &by, AddedByLabel: &label, AddedAt: now.UTC(),
	}, nil
}

// ValidateCandidate memastikan user boleh ditambahkan sebagai anggota.
func ValidateCandidate(u getuser.Response) error {
	switch {
	case !u.Found:
		return UserNotEligible(ReasonUserNotFound)
	case u.Role != resolvesession.RoleDeveloper:
		return UserNotEligible(ReasonUserNotDeveloper)
	case !u.IsActive:
		return UserNotEligible(ReasonUserInactive)
	}
	return nil
}

func OwnerCount(ms []Member) int {
	n := 0
	for _, m := range ms {
		if m.Role == RoleOwner {
			n++
		}
	}
	return n
}

func FindMember(ms []Member, userID string) (Member, bool) {
	for _, m := range ms {
		if m.UserID == userID {
			return m, true
		}
	}
	return Member{}, false
}

// ToDTO menggabungkan baris anggota dengan hasil get-user. u.Found=false -> name/email null.
func (m Member) ToDTO(u getuser.Response) MemberDTO {
	d := MemberDTO{
		UserID: m.UserID, Role: m.Role, AddedAt: m.AddedAt.UTC(),
		AddedBy: m.AddedBy, AddedByLabel: m.AddedByLabel,
	}
	if u.Found {
		name, email := u.Name, u.Email
		d.Name, d.Email, d.Found, d.IsActive = &name, &email, true, u.IsActive
	}
	return d
}
