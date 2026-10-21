package base

// CheckRoleChange memeriksa perubahan role anggota (lihat plan 3.6).
func CheckRoleChange(actorID string, actorLevel Level, target Member, newRole string, ownerCount int) error {
	if !ValidMemberRole(newRole) {
		return InvalidFields(FieldErrors{"role": "role harus owner atau member"})
	}
	if newRole == target.Role {
		return nil
	}
	if !CanManageMembers(actorLevel) {
		return ErrForbidden
	}
	if target.Role == RoleOwner && newRole == RoleMember && ownerCount <= 1 && actorLevel != LevelAdmin {
		return ErrLastOwner
	}
	return nil
}

// CheckRemove memeriksa pengeluaran/keluar anggota (lihat plan 3.6).
func CheckRemove(actorID string, actorLevel Level, target Member, ownerCount int) error {
	self := target.UserID == actorID
	if !self && !CanManageMembers(actorLevel) {
		return ErrForbidden
	}
	if target.Role == RoleOwner && ownerCount <= 1 && actorLevel != LevelAdmin {
		return ErrLastOwner
	}
	return nil
}
