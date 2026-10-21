// Package verifykey: handler bus project.apikey.verify (Call).
package verifykey

import (
	"context"
	"fmt"

	"grimoire/internal/domain/project/internal/apikey/base"
	"grimoire/lib/contracts/project/apikey/verify"
)

// outcome adalah variabel paket agar tes struktural dapat memastikan ia selalu dipanggil.
var outcome = base.VerifyOutcome

// Repository: sisi consumer. Tidak ketemu -> (zero, false, nil).
type Repository interface {
	FindByPrefix(ctx context.Context, prefix string) (base.Key, bool, error)
}

type Usecase struct{ repo Repository }

func NewUsecase(repo Repository) *Usecase { return &Usecase{repo: repo} }

// Execute memverifikasi raw key. Error DB dikembalikan sebagai error (fail-closed).
func (u *Usecase) Execute(ctx context.Context, in verify.Request) (verify.Response, error) {
	prefix, secret, ok := base.ParseRawKey(in.RawKey)
	if !ok {
		return verify.Response{Found: false, Reason: verify.ReasonMalformed}, nil
	}
	k, found, err := u.repo.FindByPrefix(ctx, prefix)
	if err != nil {
		return verify.Response{}, fmt.Errorf("verifykey: find by prefix: %w", err)
	}
	// Tanpa syarat, juga saat !found: jalur unknown membayar hash+compare yang sama.
	good, reason := outcome(k, found, secret)
	if !good {
		return verify.Response{Found: false, Prefix: prefix, Reason: reason}, nil
	}
	return verify.Response{Found: true, KeyID: k.ID, KeyName: k.Name, Prefix: k.Prefix, ProjectID: k.ProjectID}, nil
}
