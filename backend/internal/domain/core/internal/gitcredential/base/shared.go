package base

import (
	"grimoire/internal/infra/crypto/secretbox"
	"grimoire/internal/infra/gitprovider"
)

// Shared dibangun sekali di gitcredential/base.go dan diteruskan ke Init tiap feature.
type Shared struct {
	Enabled bool                // cfg.GitFeatureEnabled()
	Box     *secretbox.Box      // nil bila !Enabled
	Git     *gitprovider.Client // policy + VerifyToken
	Clock   Clock
}
