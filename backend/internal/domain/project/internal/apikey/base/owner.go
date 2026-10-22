package base

import "grimoire/lib/contracts/core/auth/resolvesession"

// IsSuperAdmin: aturan peran tunggal untuk module api-key.
func (a Actor) IsSuperAdmin() bool { return a.Role == resolvesession.RoleSuperAdmin }

// OwnerScope menentukan key mana yang boleh dilihat/dicabut actor di sebuah project.
// Sengaja struct (bukan string kosong = semua) agar ID kosong tidak pernah berarti "semua".
type OwnerScope struct {
	All     bool   // true hanya untuk super_admin
	OwnerID string // dipakai bila !All
}

// OwnerScope: super_admin -> {All: true}; selain itu -> {OwnerID: a.ID}.
func (a Actor) OwnerScope() OwnerScope {
	if a.IsSuperAdmin() {
		return OwnerScope{All: true}
	}
	return OwnerScope{OwnerID: a.ID}
}

// Permits: All -> true; selain itu hanya key dengan CreatedBy == OwnerID.
// Key legacy (CreatedBy nil) hanya lolos untuk All.
func (s OwnerScope) Permits(k Key) bool {
	if s.All {
		return true
	}
	return s.OwnerID != "" && k.CreatedBy != nil && *k.CreatedBy == s.OwnerID
}

// Empty: scope tidak bisa mengizinkan apa pun (fail-closed).
func (s OwnerScope) Empty() bool { return !s.All && s.OwnerID == "" }
