package getme

import (
	"context"

	"grimoire/internal/domain/core/internal/auth/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/common/reqctx"
)

// Output adalah respons GET /api/auth/me: MeDTO (JSON datar) ditambah penanda email.
// Dibuat khusus feature ini agar MeDTO yang dipakai feature lain tidak berubah.
type Output struct {
	base.MeDTO
	// EmailEnabled true bila server dapat mengirim email (driver smtp atau log).
	EmailEnabled bool `json:"email_enabled"`
}

// mailStatus adalah bagian Mailer yang dibutuhkan (hanya membaca flag).
type mailStatus interface{ Configured() bool }

// Usecase mengembalikan profil sesi saat ini; hanya membaca flag mailer (tanpa I/O).
type Usecase struct{ mail mailStatus }

// NewUsecase membuat Usecase. m boleh nil (dianggap email nonaktif).
func NewUsecase(m mailStatus) *Usecase { return &Usecase{mail: m} }

// Execute mengembalikan profil dari identitas sesi pada context.
// Tanpa identitas -> errs.CodeUnauthorized.
func (u *Usecase) Execute(ctx context.Context) (Output, error) {
	id, ok := reqctx.IdentityFrom(ctx)
	if !ok {
		return Output{}, errs.New(errs.CodeUnauthorized, "perlu login")
	}
	return Output{MeDTO: base.MeFromIdentity(id), EmailEnabled: u.mail != nil && u.mail.Configured()}, nil
}
