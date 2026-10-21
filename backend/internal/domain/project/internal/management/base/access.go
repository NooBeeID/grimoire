package base

import (
	"grimoire/lib/contracts/core/auth/resolvesession"
	"grimoire/lib/contracts/project/management/checkaccess"
)

type Level = checkaccess.Level

const (
	LevelNone   = checkaccess.LevelNone
	LevelMember = checkaccess.LevelMember
	LevelOwner  = checkaccess.LevelOwner
	LevelAdmin  = checkaccess.LevelAdmin
)

// ResolveLevel menentukan level akses dari role user dan role keanggotaan.
func ResolveLevel(userRole, membershipRole string, userActive bool) Level {
	if userRole == resolvesession.RoleSuperAdmin {
		return LevelAdmin
	}
	if userRole != resolvesession.RoleDeveloper {
		return LevelNone
	}
	if membershipRole == "" || !userActive {
		return LevelNone
	}
	switch membershipRole {
	case RoleOwner:
		return LevelOwner
	case RoleMember:
		return LevelMember
	}
	return LevelNone
}

type Permission int

const (
	PermView Permission = iota + 1
	PermEdit
	PermManageMembers
	PermDelete
)

// Require: none/level tak dikenal -> ErrProjectNotFound; member untuk ManageMembers/Delete -> ErrForbidden.
func Require(l Level, p Permission) error {
	switch l {
	case LevelAdmin, LevelOwner:
		return nil
	case LevelMember:
		switch p {
		case PermView, PermEdit:
			return nil
		}
		return ErrForbidden
	}
	return ErrProjectNotFound
}

// CanManageMembers: owner atau admin.
func CanManageMembers(l Level) bool { return l == LevelOwner || l == LevelAdmin }
