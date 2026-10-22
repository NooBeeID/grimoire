// Package gitcredential adalah module core/git-credential: PAT Git per user (terenkripsi),
// dibuka untuk module lain hanya lewat bus core.gitcredential.get-token.
//
// Foundation hanya mendaftarkan kode error dan merakit Shared. Feature (savecredential,
// listcredentials, deletecredential, bustoken) ditambahkan tahap wiring: satu baris Init per
// feature di Register. Route memakai pola penuh di d.API (mis. Put("/me/git-credentials/{provider}")).
package gitcredential

import (
	"net/http"
	"sync"

	"grimoire/internal/domain/core/internal/gitcredential/base"
	"grimoire/internal/domain/core/internal/gitcredential/bustoken"
	"grimoire/internal/domain/core/internal/gitcredential/deletecredential"
	"grimoire/internal/domain/core/internal/gitcredential/listcredentials"
	"grimoire/internal/domain/core/internal/gitcredential/savecredential"
	"grimoire/internal/infra/app"
	"grimoire/internal/infra/crypto/secretbox"
	"grimoire/internal/infra/gitprovider"
	httpx "grimoire/internal/infra/http"
)

var codesOnce sync.Once

// registerCodes mendaftarkan status HTTP kode error milik module ini. Dibungkus sync.Once
// karena RegisterCodeStatus panic bila duplikat. Kode bersama (HOST_NOT_ALLOWED, GIT_UPSTREAM_ERROR,
// GIT_FEATURE_DISABLED) sudah ada di peta bawaan.
func registerCodes() {
	codesOnce.Do(func() {
		httpx.RegisterCodeStatus(base.CodeTokenInvalid, http.StatusUnprocessableEntity)
	})
}

// newShared merakit dependensi bersama dari config. Box nil bila fitur nonaktif (kunci kosong).
// Kunci sudah divalidasi config, jadi kegagalan di sini = bug wiring (panic, rule 5.9).
func newShared(d app.Deps) base.Shared {
	sh := base.Shared{
		Enabled: d.Config.GitFeatureEnabled(),
		Git:     gitprovider.NewFromConfig(d.Config),
		Clock:   base.SystemClock,
	}
	if sh.Enabled {
		box, err := secretbox.New(d.Config.GitCredentialKey)
		if err != nil {
			panic("gitcredential: " + err.Error())
		}
		sh.Box = box
	}
	return sh
}

// Register merakit module.
func Register(d app.Deps) {
	registerCodes()
	sh := newShared(d)

	// Handler bus.
	bustoken.Init(d, sh)

	// Route REST /api/me/git-credentials (pola penuh untuk PUT, GET, dan DELETE).
	savecredential.Init(d, sh)
	listcredentials.Init(d)
	deletecredential.Init(d)
}
