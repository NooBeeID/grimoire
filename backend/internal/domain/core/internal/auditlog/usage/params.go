// Package usage adalah feature GET /api/audit-log/usage (T19): insight pemakaian agent.
package usage

import (
	"net/url"
	"strings"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

// ParseQuery hanya membaca BENTUK: nilai di-TrimSpace; parameter lain diabaikan.
// Nilai divalidasi di base.UsageQuery.Validate.
func ParseQuery(q url.Values) base.UsageQuery {
	return base.UsageQuery{
		Range:     base.Range(strings.TrimSpace(q.Get("range"))),
		ProjectID: strings.TrimSpace(q.Get("project_id")),
		Section:   base.Section(strings.TrimSpace(q.Get("section"))),
	}
}
