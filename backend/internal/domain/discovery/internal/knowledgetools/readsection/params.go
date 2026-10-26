package readsection

import (
	"encoding/json"
	"fmt"
	"strings"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

const (
	maxHeadingBytes = 500
	maxOccurrence   = 1000
	pathHint        = "Dapatkan path dengan list_files atau search_knowledge."
	headingHint     = "Dapatkan daftar heading dengan get_file_outline."
)

type rawArgs struct {
	Path       *string `json:"path"`
	Heading    *string `json:"heading"`
	Occurrence *int    `json:"occurrence"`
	Limit      *int    `json:"limit"`
}

var allowedArgs = []string{"path", "heading", "occurrence", "limit"}

// Params adalah argumen read_section yang sudah tervalidasi.
type Params struct {
	Path, Heading string
	Occurrence    int // default 1, 1..1000
	Limit         int // default base.DefaultReadLimit(max), 1..max
}

func schema(maxLimit int) json.RawMessage {
	s := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"path", "heading"},
		"properties": map[string]any{
			"path": map[string]any{
				"type": "string", "minLength": 1, "maxLength": base.MaxPathBytes,
				"description": "path file Markdown, mis. docs/guides/a.md",
			},
			"heading": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxHeadingBytes,
				"description": "teks heading persis seperti di get_file_outline, tanpa tanda #",
			},
			"occurrence": map[string]any{
				"type": "integer", "minimum": 1, "maximum": maxOccurrence, "default": 1,
				"description": "kemunculan ke-n bila ada beberapa heading bernama sama",
			},
			"limit": map[string]any{
				"type": "integer", "minimum": 1, "maximum": maxLimit, "default": base.DefaultReadLimit(maxLimit),
				"description": "jumlah byte maksimal yang dikembalikan",
			},
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic("readsection: schema: " + err.Error())
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
		return Params{}, e
	}
	if r.Heading == nil || strings.TrimSpace(*r.Heading) == "" {
		return Params{}, &base.ArgError{Field: "heading", Reason: "wajib diisi", Hint: headingHint}
	}
	heading := strings.TrimSpace(*r.Heading)
	if len(heading) > maxHeadingBytes {
		return Params{}, &base.ArgError{Field: "heading", Reason: "maksimal 500 byte", Hint: headingHint}
	}
	occ, e := base.IntArg("occurrence", r.Occurrence, 1, 1, maxOccurrence,
		"Pakai 1 untuk kemunculan pertama.")
	if e != nil {
		return Params{}, e
	}
	limit, e := base.IntArg("limit", r.Limit, base.DefaultReadLimit(maxLimit), 1, maxLimit,
		fmt.Sprintf("Maksimum %d byte per panggilan; ambil sisanya dengan read_file offset dari penanda.", maxLimit))
	if e != nil {
		return Params{}, e
	}
	return Params{Path: path, Heading: heading, Occurrence: occ, Limit: limit}, nil
}
