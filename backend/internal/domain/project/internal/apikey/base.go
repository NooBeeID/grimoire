// Package apikey merakit module project/api-key: route HTTP dan handler bus.
package apikey

import (
	"grimoire/internal/domain/project/internal/apikey/countkeys"
	"grimoire/internal/domain/project/internal/apikey/createkey"
	"grimoire/internal/domain/project/internal/apikey/listkeys"
	"grimoire/internal/domain/project/internal/apikey/revokekey"
	"grimoire/internal/domain/project/internal/apikey/touchlastused"
	"grimoire/internal/domain/project/internal/apikey/verifykey"
	"grimoire/internal/infra/app"
)

// Register merakit module: handler bus dan route REST (pola penuh di d.API).
func Register(d app.Deps) {
	// Handler bus.
	verifykey.Init(d)
	touchlastused.Init(d)
	countkeys.Init(d)

	// Route REST /api/projects/{projectID}/api-keys...
	createkey.Init(d)
	listkeys.Init(d)
	revokekey.Init(d)
}
