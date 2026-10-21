// Package scanentries adalah feature bus core.audit.scan-entries.
package scanentries

import (
	"time"

	"github.com/google/uuid"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/scanentries"
)

type params struct {
	ProjectID string
	Actions   []string
	From, To  time.Time
	After     *scanentries.Cursor
	Limit     int
}

func invalidField(name string) error {
	e := errs.WithDetails(errs.CodeInvalid, name+" tidak valid", map[string]string{"field": name})
	e.Err = errs.ErrInvalid
	return e
}

// canonicalUUID mengembalikan bentuk kanonik huruf kecil; ok=false bila input bukan uuid kanonik.
func canonicalUUID(s string) (string, bool) {
	u, err := uuid.Parse(s)
	if err != nil || u.String() != s {
		return "", false
	}
	return s, true
}

// parse memvalidasi Request dan menerapkan default. now dipakai bila To nol.
func parse(in scanentries.Request, now time.Time) (params, error) {
	var p params
	pid, ok := canonicalUUID(in.ProjectID)
	if !ok {
		return params{}, invalidField("project_id")
	}
	p.ProjectID = pid

	seen := make(map[string]struct{}, len(in.Actions))
	for _, a := range in.Actions {
		if !base.ValidAction(a) {
			return params{}, invalidField("actions")
		}
		if _, dup := seen[a]; dup {
			continue
		}
		seen[a] = struct{}{}
		p.Actions = append(p.Actions, a)
	}
	if len(p.Actions) < 1 || len(p.Actions) > scanentries.MaxActions {
		return params{}, invalidField("actions")
	}

	if in.From.IsZero() {
		return params{}, invalidField("from")
	}
	p.From = in.From.UTC()

	p.To = in.To.UTC()
	if in.To.IsZero() {
		p.To = now.UTC()
	}
	if !p.To.After(p.From) {
		return params{}, invalidField("to")
	}

	switch {
	case in.Limit == 0:
		p.Limit = scanentries.DefaultLimit
	case in.Limit < 0 || in.Limit > scanentries.MaxLimit:
		return params{}, invalidField("limit")
	default:
		p.Limit = in.Limit
	}

	if in.After != nil {
		id, ok := canonicalUUID(in.After.ID)
		if in.After.OccurredAt.IsZero() || !ok {
			return params{}, invalidField("after")
		}
		p.After = &scanentries.Cursor{OccurredAt: in.After.OccurredAt.UTC(), ID: id}
	}
	return p, nil
}
