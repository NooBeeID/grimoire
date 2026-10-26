package getfileoutline

import (
	"encoding/json"
	"strings"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

type rawArgs struct {
	Path *string `json:"path"`
}

var allowedArgs = []string{"path"}

const pathHint = "Dapatkan path dengan list_files atau search_knowledge."

// Params adalah argumen get_file_outline yang sudah tervalidasi.
type Params struct {
	Path string // ternormalisasi
}

func schema() json.RawMessage {
	s := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"path"},
		"properties": map[string]any{
			"path": map[string]any{
				"type": "string", "minLength": 1, "maxLength": base.MaxPathBytes,
				"description": "path file Markdown, mis. docs/guides/a.md",
			},
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic("getfileoutline: schema: " + err.Error())
	}
	return b
}

// Parse memvalidasi argumen.
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var r rawArgs
	if e := base.DecodeStrict(raw, &r, allowedArgs); e != nil {
		return Params{}, e
	}
	if r.Path == nil || strings.TrimSpace(*r.Path) == "" {
		return Params{}, &base.ArgError{Field: "path", Reason: "wajib diisi", Hint: pathHint}
	}
	path, e := base.NormalizePath("path", *r.Path)
	if e != nil {
		return Params{}, e
	}
	return Params{Path: path}, nil
}
