package listapiendpoints

import (
	"encoding/json"

	"grimoire/internal/domain/discovery/internal/apispectools/base"
)

type rawArgs struct {
	Spec   *string `json:"spec"`
	Tag    *string `json:"tag"`
	Query  *string `json:"query"`
	Limit  *int    `json:"limit"`
	Offset *int    `json:"offset"`
}

// Params hasil validasi. Tidak memuat project.
type Params struct {
	Spec, Tag, Query string
	Limit            int
	Offset           int
}

const (
	maxSpecRunes  = 80
	maxTagRunes   = 200
	maxQueryRunes = 200 // = listendpoints.MaxQueryRunes
	defaultLimit  = 100 // = listendpoints.DefaultLimit
	maxLimit      = 300 // = listendpoints.MaxLimit
	maxOffset     = 1_000_000
)

var allowedFields = []string{"spec", "tag", "query", "limit", "offset"}

func schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "spec":   { "type": "string", "maxLength": 80, "description": "nama spec dari list_api_specs; kosong = semua spec" },
    "tag":    { "type": "string", "maxLength": 200, "description": "tag persis (peka huruf besar/kecil)" },
    "query":  { "type": "string", "maxLength": 200, "description": "kata yang dicari pada path, ringkasan, atau operationId" },
    "limit":  { "type": "integer", "minimum": 1, "maximum": 300, "default": 100, "description": "jumlah endpoint per halaman" },
    "offset": { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah endpoint; pakai nilai dari penanda [ada N endpoint lagi ...]" }
  },
  "additionalProperties": false
}`)
}

// Parse memvalidasi argumen tool; error pertama menang (urutan field sesuai schema).
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var a rawArgs
	if err := base.DecodeStrict(raw, &a, allowedFields); err != nil {
		return Params{}, err
	}
	var p Params
	var err *base.ArgError
	if p.Spec, err = base.StrArg("spec", a.Spec, maxSpecRunes, "Panggil list_api_specs untuk melihat nama spec yang valid."); err != nil {
		return Params{}, err
	}
	if p.Tag, err = base.StrArg("tag", a.Tag, maxTagRunes, "Panggil list_api_endpoints tanpa tag untuk melihat tag yang ada."); err != nil {
		return Params{}, err
	}
	if p.Query, err = base.StrArg("query", a.Query, maxQueryRunes, "Persingkat kata kunci."); err != nil {
		return Params{}, err
	}
	if p.Limit, err = base.IntArg("limit", a.Limit, defaultLimit, 1, maxLimit, "Gunakan offset untuk halaman berikutnya."); err != nil {
		return Params{}, err
	}
	if p.Offset, err = base.IntArg("offset", a.Offset, 0, 0, maxOffset, "Gunakan nilai offset dari penanda [ada N endpoint lagi ...]."); err != nil {
		return Params{}, err
	}
	return p, nil
}
