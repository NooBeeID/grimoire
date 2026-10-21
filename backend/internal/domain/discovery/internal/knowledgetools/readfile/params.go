package readfile

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

type rawArgs struct {
	Path   *string `json:"path"`
	Offset *int    `json:"offset"`
	Limit  *int    `json:"limit"`
}

var allowedArgs = []string{"path", "offset", "limit"}

const pathHint = "Dapatkan path dengan list_files atau search_knowledge."

// Params adalah argumen read_file yang sudah tervalidasi.
type Params struct {
	Path   string // ternormalisasi
	Offset int
	Limit  int
}

// schema membangun InputSchema dengan batas limit dari config.
func schema(maxLimit int) json.RawMessage {
	s := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"path"},
		"properties": map[string]any{
			"path": map[string]any{
				"type": "string", "minLength": 1, "maxLength": base.MaxPathBytes,
				"description": "path file, mis. docs/guides/a.md",
			},
			"offset": map[string]any{
				"type": "integer", "minimum": 0, "maximum": math.MaxInt32, "default": 0,
				"description": "posisi byte awal; pakai nilai dari penanda terpotong",
			},
			"limit": map[string]any{
				"type": "integer", "minimum": 1, "maximum": maxLimit, "default": base.DefaultReadLimit(maxLimit),
				"description": "jumlah byte maksimal per potongan",
			},
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic("readfile: schema: " + err.Error())
	}
	return b
}

// Parse memvalidasi argumen. Error pertama menang, urutan field sesuai schema.
func Parse(raw json.RawMessage, maxLimit int) (Params, *base.ArgError) {
	var r rawArgs
	if e := base.DecodeStrict(raw, &r, allowedArgs); e != nil {
		return Params{}, e
	}
	if r.Path == nil || strings.TrimSpace(*r.Path) == "" {
		return Params{}, &base.ArgError{Field: "path", Reason: "wajib diisi", Hint: pathHint}
	}
	path, e := base.NormalizePath("path", *r.Path)
	if e != nil {
		if e.Reason == "wajib diisi" && e.Hint == "" {
			e.Hint = pathHint
		}
		return Params{}, e
	}
	offset, e := base.IntArg("offset", r.Offset, 0, 0, math.MaxInt32,
		"Pakai offset dari penanda [terpotong: ...] atau 0.")
	if e != nil {
		return Params{}, e
	}
	limit, e := base.IntArg("limit", r.Limit, base.DefaultReadLimit(maxLimit), 1, maxLimit,
		fmt.Sprintf("Maksimum %d byte per panggilan; ambil sisanya dengan offset dari penanda.", maxLimit))
	if e != nil {
		return Params{}, e
	}
	return Params{Path: path, Offset: offset, Limit: limit}, nil
}
