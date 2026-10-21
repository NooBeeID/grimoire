// Package base berisi entity dan rule bisnis module auth. Murni: tanpa I/O, SQL, atau context.
package base

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"grimoire/lib/common/reqctx"
	"grimoire/lib/contracts/core/auth/resolvesession"
)

const (
	RoleSuperAdmin = resolvesession.RoleSuperAdmin
	RoleDeveloper  = resolvesession.RoleDeveloper

	MaxEmailBytes = 254
	MaxNameRunes  = 100
	minEmailBytes = 3
)

const (
	msgRequired    = "wajib diisi"
	msgEmailFormat = "format email tidak valid"
	msgRole        = "role harus super_admin atau developer"
)

// User adalah entity akun. Tanpa tag db: pemetaan baris ada di repository.
type User struct {
	ID                 string
	Email              string
	Name               string
	PasswordHash       string
	Role               string
	IsActive           bool
	MustChangePassword bool
	CreatedAt          time.Time
	CreatedBy          *string
	CreatedByLabel     *string
	UpdatedAt          time.Time
	LastLoginAt        *time.Time
}

// UserDTO adalah bentuk respons user (tanpa hash).
type UserDTO struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	CreatedBy          *string    `json:"created_by"`
	CreatedByLabel     *string    `json:"created_by_label"`
	UpdatedAt          time.Time  `json:"updated_at"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	// Invitation selalu hadir di JSON; null bila tidak ada undangan menunggu.
	Invitation *InvitationDTO `json:"invitation"`
}

// MeDTO adalah profil ringkas user yang sedang login.
type MeDTO struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

// NormalizeEmail: trim + lowercase; 3..254 byte; harus alamat polos (tanpa nama tampilan).
// Gagal: FieldErrors{"email": ...}.
func NormalizeEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", fieldErr("email", msgRequired)
	}
	if len(s) < minEmailBytes || len(s) > MaxEmailBytes {
		return "", fieldErr("email", msgEmailFormat)
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return "", fieldErr("email", msgEmailFormat)
	}
	return s, nil
}

// ValidateName: trim; 1..100 rune; tanpa karakter kontrol. Gagal: FieldErrors{"name": ...}.
func ValidateName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fieldErr("name", msgRequired)
	}
	if utf8.RuneCountInString(s) > MaxNameRunes {
		return "", fieldErr("name", "maksimal 100 karakter")
	}
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", fieldErr("name", "tidak boleh berisi karakter kontrol")
	}
	return s, nil
}

// ValidRole: super_admin atau developer.
func ValidRole(r string) bool { return r == RoleSuperAdmin || r == RoleDeveloper }

// CanLogin: hanya user aktif yang boleh login / punya sesi valid.
func (u User) CanLogin() bool { return u.IsActive }

// ToResponse membentuk DTO tanpa PasswordHash.
func (u User) ToResponse() UserDTO {
	return UserDTO{
		ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, IsActive: u.IsActive,
		MustChangePassword: u.MustChangePassword, CreatedAt: u.CreatedAt, CreatedBy: u.CreatedBy,
		CreatedByLabel: u.CreatedByLabel, UpdatedAt: u.UpdatedAt, LastLoginAt: u.LastLoginAt,
	}
}

// UserWithInvitation menggandeng user dan undangan menunggu (nil bila tidak ada).
type UserWithInvitation struct {
	User       User
	Invitation *Invitation
}

// ToResponseWithInvitation seperti ToResponse dengan penanda undangan (inv nil -> null).
func (u User) ToResponseWithInvitation(inv *Invitation, now time.Time) UserDTO {
	dto := u.ToResponse()
	if inv != nil {
		dto.Invitation = inv.ToResponse(now)
	}
	return dto
}

// ToMe membentuk profil ringkas.
func (u User) ToMe() MeDTO {
	return MeDTO{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, MustChangePassword: u.MustChangePassword}
}

// MeFromIdentity memetakan identitas sesi (dari reqctx) ke MeDTO.
func MeFromIdentity(id reqctx.Identity) MeDTO {
	return MeDTO{ID: id.UserID, Email: id.Email, Name: id.Name, Role: id.Role, MustChangePassword: id.MustChangePassword}
}

// UserPatch adalah perubahan parsial; nil = tidak diubah.
type UserPatch struct{ Name, Email, Role *string }

// ApplyUpdate memvalidasi dan menerapkan patch secara atomik (gagal = u tidak berubah).
// Field yang nilai ternormalisasinya sama dengan nilai tersimpan tidak dihitung berubah.
// changed berurutan dari ["name","email","role"]. err bertipe FieldErrors bila ada yang gagal.
func (u *User) ApplyUpdate(p UserPatch) (changed []string, err error) {
	fe := FieldErrors{}
	var name, email, role string
	var setName, setEmail, setRole bool

	if p.Name != nil {
		v, e := ValidateName(*p.Name)
		if e != nil {
			fe = fe.Merge(AsFieldErrors(e))
		} else if v != u.Name {
			name, setName = v, true
		}
	}
	if p.Email != nil {
		v, e := NormalizeEmail(*p.Email)
		if e != nil {
			fe = fe.Merge(AsFieldErrors(e))
		} else if v != strings.ToLower(u.Email) {
			email, setEmail = v, true
		}
	}
	if p.Role != nil {
		if !ValidRole(*p.Role) {
			fe["role"] = msgRole
		} else if *p.Role != u.Role {
			role, setRole = *p.Role, true
		}
	}
	if len(fe) > 0 {
		return nil, fe
	}
	if setName {
		u.Name = name
		changed = append(changed, "name")
	}
	if setEmail {
		u.Email = email
		changed = append(changed, "email")
	}
	if setRole {
		u.Role = role
		changed = append(changed, "role")
	}
	return changed, nil
}

// ApplyStatus mengubah status aktif; true bila berubah. Status sama: u tidak berubah.
func (u *User) ApplyStatus(active bool, now time.Time) bool {
	if u.IsActive == active {
		return false
	}
	u.IsActive = active
	u.UpdatedAt = now
	return true
}

// WithResetPassword mengembalikan salinan dengan hash baru dan wajib ganti password.
func (u User) WithResetPassword(hash string, now time.Time) User {
	u.PasswordHash = hash
	u.MustChangePassword = true
	u.UpdatedAt = now
	return u
}

// NewUserInput adalah input pembuatan user baru.
type NewUserInput struct {
	Email, Name, Role         string
	CreatedBy, CreatedByLabel string
	Now                       time.Time
}

// NewUser memvalidasi input dan membentuk entity siap insert (tanpa ID dan PasswordHash).
// Semua kesalahan field dikumpulkan. Hasil: aktif, wajib ganti password.
func NewUser(in NewUserInput) (User, FieldErrors) {
	var fe FieldErrors
	email, err := NormalizeEmail(in.Email)
	fe = fe.Merge(AsFieldErrors(err))
	name, err := ValidateName(in.Name)
	fe = fe.Merge(AsFieldErrors(err))
	if !ValidRole(in.Role) {
		fe = fe.Merge(FieldErrors{"role": msgRole})
	}
	if len(fe) > 0 {
		return User{}, fe
	}
	u := User{
		Email: email, Name: name, Role: in.Role, IsActive: true, MustChangePassword: true,
		CreatedAt: in.Now, UpdatedAt: in.Now,
	}
	if in.CreatedBy != "" {
		cb := in.CreatedBy
		u.CreatedBy = &cb
	}
	if in.CreatedByLabel != "" {
		l := in.CreatedByLabel
		u.CreatedByLabel = &l
	}
	return u, nil
}
