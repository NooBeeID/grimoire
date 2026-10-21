package searchknowledge

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

const (
	maxQueryRunes = 200
	defaultLimit  = 10
	maxLimit      = 20
	maxPathBytes  = 1024
)

// Params adalah argumen tervalidasi.
type Params struct {
	Query  string // sudah TrimSpace, 1-200 rune, karakter khusus apa adanya
	Prefix string // base.NormalizePrefix; "" = akar
	Limit  int    // 1-20
}

type rawArgs struct {
	Query *string `json:"query"`
	Path  *string `json:"path"`
	Limit *int    `json:"limit"`
}

var allowedArgs = []string{"query", "path", "limit"}

func schema() json.RawMessage {
	b, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "minLength": 1, "maxLength": maxQueryRunes,
				"description": "kata kunci atau frasa dalam tanda kutip, mis. \"rate limit\""},
			"path": map[string]any{"type": "string", "maxLength": maxPathBytes,
				"description": "prefix folder opsional, mis. docs/guides"},
			"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxLimit, "default": defaultLimit,
				"description": "jumlah hasil maksimal"},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	})
	if err != nil {
		panic("searchknowledge: schema: " + err.Error())
	}
	return b
}

// Parse memvalidasi argumen; hanya error pertama (urutan schema) yang dilaporkan.
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var ra rawArgs
	if e := base.DecodeStrict(raw, &ra, allowedArgs); e != nil {
		return Params{}, e
	}
	q := ""
	if ra.Query != nil {
		q = strings.TrimSpace(*ra.Query)
	}
	if q == "" {
		return Params{}, &base.ArgError{Field: "query", Reason: "wajib diisi 1-200 karakter", Hint: "Contoh: query=\"rate limit\"."}
	}
	if utf8.RuneCountInString(q) > maxQueryRunes {
		return Params{}, &base.ArgError{Field: "query", Reason: "maksimal 200 karakter", Got: base.Brief(q),
			Hint: "Ringkas menjadi kata kunci utama, atau pecah menjadi beberapa pencarian."}
	}
	rawPath := ""
	if ra.Path != nil {
		rawPath = *ra.Path
	}
	prefix, e := base.NormalizePrefix("path", rawPath)
	if e != nil {
		return Params{}, e
	}
	limit, e := base.IntArg("limit", ra.Limit, defaultLimit, 1, maxLimit,
		"Persempit dengan path=<folder> atau ubah kata kunci, bukan menaikkan limit.")
	if e != nil {
		return Params{}, e
	}
	return Params{Query: q, Prefix: prefix, Limit: limit}, nil
}
