package listrecentchanges

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/contracts/project/knowledge/listchanges"
)

const (
	KindFiles = "files"
	KindSpecs = "specs"
	KindAll   = "all"

	maxSinceBytes = 64
	sinceHint     = "Contoh: \"24h\", \"7d\", atau \"2026-10-20T00:00:00Z\" (maksimal 90 hari ke belakang)."
)

var allowedArgs = []string{"since", "kind", "path", "limit"}

var relSince = regexp.MustCompile(`^([1-9][0-9]{0,5})(h|d)$`)

type rawArgs struct {
	Since *string `json:"since"`
	Kind  *string `json:"kind"`
	Path  *string `json:"path"`
	Limit *int    `json:"limit"`
}

// Params adalah argumen list_recent_changes yang sudah tervalidasi.
type Params struct {
	Since  time.Time // UTC
	Kind   string
	Prefix string
	Limit  int
}

func schema() json.RawMessage {
	s := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"since"},
		"properties": map[string]any{
			"since": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxSinceBytes,
				"description": "batas waktu awal: RFC3339 (2026-10-20T00:00:00Z) atau durasi relatif 24h, 7d, 30d; maksimal 90 hari ke belakang",
			},
			"kind": map[string]any{
				"type": "string", "enum": []string{KindFiles, KindSpecs, KindAll}, "default": KindAll,
				"description": "files = file knowledge, specs = spec API, all = keduanya",
			},
			"path": map[string]any{
				"type": "string", "maxLength": base.MaxPathBytes,
				"description": "prefix path file knowledge (hanya untuk kind files atau all)",
			},
			"limit": map[string]any{
				"type": "integer", "minimum": 1, "maximum": listchanges.MaxLimit, "default": listchanges.DefaultLimit,
				"description": "jumlah perubahan maksimal",
			},
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic("listrecentchanges: schema: " + err.Error())
	}
	return b
}

// ParseSince menerima RFC3339 atau <n>h / <n>d. Hasil UTC; lebih lama dari 90 hari atau di masa depan ditolak.
func ParseSince(raw string, now time.Time) (time.Time, *base.ArgError) {
	raw = strings.TrimSpace(raw)
	bad := func(reason string) *base.ArgError {
		return &base.ArgError{Field: "since", Reason: reason, Got: base.Brief(raw), Hint: sinceHint}
	}
	var since time.Time
	if m := relSince.FindStringSubmatch(raw); m != nil {
		n, _ := strconv.Atoi(m[1])
		d := time.Duration(n) * time.Hour
		if m[2] == "d" {
			d *= 24
		}
		if d > base.MaxChangesWindow {
			return time.Time{}, bad("melebihi jendela 90 hari")
		}
		since = now.Add(-d)
	} else if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		since = t
	} else {
		return time.Time{}, bad("bukan RFC3339 atau durasi <n>h/<n>d")
	}
	since = since.UTC()
	if since.After(now) {
		return time.Time{}, bad("tidak boleh di masa depan")
	}
	if now.Sub(since) > base.MaxChangesWindow {
		return time.Time{}, bad("melebihi jendela 90 hari")
	}
	return since, nil
}

// Parse memvalidasi argumen. Error pertama menang, urutan field sesuai schema.
func Parse(raw json.RawMessage, now time.Time) (Params, *base.ArgError) {
	var r rawArgs
	if e := base.DecodeStrict(raw, &r, allowedArgs); e != nil {
		return Params{}, e
	}
	if r.Since == nil || strings.TrimSpace(*r.Since) == "" {
		return Params{}, &base.ArgError{Field: "since", Reason: "wajib diisi", Hint: sinceHint}
	}
	if len(*r.Since) > maxSinceBytes {
		return Params{}, &base.ArgError{Field: "since", Reason: "maksimal 64 byte", Hint: sinceHint}
	}
	since, e := ParseSince(*r.Since, now)
	if e != nil {
		return Params{}, e
	}
	kind := KindAll
	if r.Kind != nil {
		kind = *r.Kind
		if kind != KindFiles && kind != KindSpecs && kind != KindAll {
			return Params{}, &base.ArgError{Field: "kind", Reason: "harus salah satu dari files, specs, all", Got: base.Brief(kind)}
		}
	}
	prefix := ""
	if r.Path != nil {
		p, e := base.NormalizePrefix("path", *r.Path)
		if e != nil {
			return Params{}, e
		}
		if p != "" && kind == KindSpecs {
			return Params{}, &base.ArgError{Field: "path", Reason: "hanya berlaku untuk kind files atau all",
				Hint: "Hapus path atau ganti kind menjadi files atau all."}
		}
		prefix = p
	}
	limit, e := base.IntArg("limit", r.Limit, listchanges.DefaultLimit, 1, listchanges.MaxLimit,
		"Persempit since, path, atau kind untuk hasil lebih sedikit.")
	if e != nil {
		return Params{}, e
	}
	return Params{Since: since, Kind: kind, Prefix: prefix, Limit: limit}, nil
}
