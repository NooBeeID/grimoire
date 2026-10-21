package listfiles

import (
	"encoding/json"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
)

type rawArgs struct {
	Path      *string `json:"path"`
	Recursive *bool   `json:"recursive"`
	Limit     *int    `json:"limit"`
	Offset    *int    `json:"offset"`
}

// Params hasil validasi. Tidak memuat project.
type Params struct {
	Prefix    string // sudah base.NormalizePrefix
	Recursive bool
	Limit     int // 1-500
	Offset    int // 0-1000000
}

const (
	defaultLimit = 200
	maxLimit     = 500
	maxOffset    = 1_000_000
)

var allowedFields = []string{"path", "recursive", "limit", "offset"}

const inputSchema = `{
  "type": "object",
  "properties": {
    "path":      { "type": "string",  "maxLength": 1024, "description": "prefix folder, mis. docs/guides; kosong = akar" },
    "recursive": { "type": "boolean", "default": false,  "description": "true = semua file di bawah path; false = satu tingkat beserta entri [folder]" },
    "limit":     { "type": "integer", "minimum": 1, "maximum": 500, "default": 200, "description": "jumlah file per halaman" },
    "offset":    { "type": "integer", "minimum": 0, "maximum": 1000000, "default": 0, "description": "lewati sejumlah file; pakai nilai dari penanda [ada N file lagi ...]" }
  },
  "additionalProperties": false
}`

func schema() json.RawMessage { return json.RawMessage(inputSchema) }

// Parse memvalidasi argumen tool secara ketat; error pertama menang.
func Parse(raw json.RawMessage) (Params, *base.ArgError) {
	var a rawArgs
	if err := base.DecodeStrict(raw, &a, allowedFields); err != nil {
		return Params{}, err
	}
	var p Params
	if a.Path != nil {
		prefix, err := base.NormalizePrefix("path", *a.Path)
		if err != nil {
			return Params{}, err
		}
		p.Prefix = prefix
	}
	p.Recursive = base.BoolArg(a.Recursive, false)
	var err *base.ArgError
	if p.Limit, err = base.IntArg("limit", a.Limit, defaultLimit, 1, maxLimit, "Gunakan offset untuk halaman berikutnya."); err != nil {
		return Params{}, err
	}
	if p.Offset, err = base.IntArg("offset", a.Offset, 0, 0, maxOffset, "Gunakan nilai offset dari penanda [ada N file lagi ...]."); err != nil {
		return Params{}, err
	}
	return p, nil
}
