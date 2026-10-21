package listapispecs

import (
	"encoding/json"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
)

// rawArgs sengaja kosong: DecodeStrict menolak setiap field.
type rawArgs struct{}

// Params tidak memuat apa pun (tool tanpa argumen).
type Params struct{}

var allowedFields = []string{}

const inputSchema = `{"type":"object","properties":{},"additionalProperties":false}`

func schema() json.RawMessage { return json.RawMessage(inputSchema) }

// Parse menolak semua argumen.
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var a rawArgs
	if e := base.DecodeStrict(raw, &a, allowedFields); e != nil {
		return Params{}, e
	}
	return Params{}, nil
}
