// Package listentries adalah feature GET /api/audit-log.
package listentries

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/domain/core/internal/auditlog/base"
	"grimoire/lib/common/errs"
)

func invalidField(name string) error {
	e := errs.WithDetails(errs.CodeInvalid, name+" tidak valid", map[string]string{"field": name})
	e.Err = errs.ErrInvalid
	return e
}

func parseTime(q url.Values, key string) (time.Time, error) {
	v := q.Get(key)
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, invalidField(key)
	}
	return t.UTC(), nil
}

// ParseFilter mengubah query string menjadi base.Filter. Hanya memeriksa BENTUK
// (angka, RFC3339, cursor terdekode). Aturan nilai ada di base.Filter.Validate.
func ParseFilter(q url.Values) (base.Filter, error) {
	f := base.Filter{
		ActorType:  strings.TrimSpace(q.Get("actor_type")),
		ActorID:    strings.TrimSpace(q.Get("actor_id")),
		ActorLabel: strings.TrimSpace(q.Get("actor_label")),
		ProjectID:  strings.TrimSpace(q.Get("project_id")),
		Target:     strings.TrimSpace(q.Get("target")),
		Actions:    q["action"],
		Statuses:   q["status"],
	}
	var err error
	if f.From, err = parseTime(q, "from"); err != nil {
		return base.Filter{}, err
	}
	if f.To, err = parseTime(q, "to"); err != nil {
		return base.Filter{}, err
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return base.Filter{}, invalidField("limit")
		}
		f.Limit = n
	}
	if v := q.Get("cursor"); v != "" {
		c, err := base.DecodeCursor(v)
		if err != nil {
			return base.Filter{}, err
		}
		f.Cursor = &c
	}
	return f, nil
}
