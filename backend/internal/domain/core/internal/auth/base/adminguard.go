package base

// Alasan (details.reason) untuk penolakan aturan penjaga super admin.
const (
	ReasonSelfAction     = "SELF_ACTION_FORBIDDEN"
	ReasonLastSuperAdmin = "LAST_SUPER_ADMIN"
)

// CheckRoleChange memeriksa perubahan role target. Urutan: self lebih dulu, lalu last-admin.
func CheckRoleChange(actorID string, target User, newRole string, activeSuperAdminCount int) error {
	if newRole == target.Role {
		return nil
	}
	if target.ID == actorID {
		return ErrSelfAction
	}
	if target.Role == RoleSuperAdmin && target.IsActive && activeSuperAdminCount <= 1 {
		return ErrLastSuperAdmin
	}
	return nil
}

// CheckDeactivate memeriksa penonaktifan target. Target yang sudah nonaktif lolos (idempoten).
func CheckDeactivate(actorID string, target User, activeSuperAdminCount int) error {
	if !target.IsActive {
		return nil
	}
	if target.ID == actorID {
		return ErrSelfAction
	}
	if target.Role == RoleSuperAdmin && activeSuperAdminCount <= 1 {
		return ErrLastSuperAdmin
	}
	return nil
}
