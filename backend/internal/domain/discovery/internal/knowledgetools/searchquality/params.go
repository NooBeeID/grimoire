// Package searchquality adalah feature bus discovery.knowledgetools.search-quality-report (T11).
package searchquality

import (
	"regexp"
	"strings"
	"time"

	"grimoire/internal/domain/discovery/internal/knowledgetools/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/discovery/knowledgetools/searchquality"
)

// Params adalah request yang sudah tervalidasi dan dinormalisasi.
type Params struct {
	ProjectID string        // uuid kanonik huruf kecil
	Since     time.Time     // UTC
	Until     time.Time     // UTC, > Since
	Window    time.Duration // MinQualityWindow..MaxQualityWindow
}

// uuidRe memeriksa bentuk uuid 8-4-4-4-12 (github.com/google/uuid tidak masuk allowlist import knowledgetools).
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func invalid(msg string) error {
	return errs.Wrap(errs.CodeInvalid, msg, errs.ErrInvalid)
}

// ParseParams memvalidasi request. now dipakai bila Until nol. Pesan error tidak memuat nilai input.
func ParseParams(in searchquality.Request, now time.Time) (Params, error) {
	if !uuidRe.MatchString(in.ProjectID) {
		return Params{}, invalid("project_id tidak valid")
	}
	if in.Since.IsZero() {
		return Params{}, invalid("since wajib diisi")
	}
	until := in.Until
	if until.IsZero() {
		until = now
	}
	if !until.After(in.Since) {
		return Params{}, invalid("until harus setelah since")
	}
	window := in.Window
	if window == 0 {
		window = base.DefaultQualityWindow
	}
	if window < base.MinQualityWindow || window > base.MaxQualityWindow {
		return Params{}, invalid("window harus 1s..1h")
	}
	return Params{ProjectID: strings.ToLower(in.ProjectID), Since: in.Since.UTC(), Until: until.UTC(), Window: window}, nil
}
