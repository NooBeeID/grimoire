// Package summary adalah feature GET /api/audit-log/summary (T15).
package summary

import (
	"net/url"
	"strings"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

// ParseQuery hanya membaca BENTUK: range dan project_id di-TrimSpace; parameter lain diabaikan.
// Nilai divalidasi di base.SummaryQuery.Validate.
func ParseQuery(q url.Values) base.SummaryQuery {
	return base.SummaryQuery{
		Range:     base.Range(strings.TrimSpace(q.Get("range"))),
		ProjectID: strings.TrimSpace(q.Get("project_id")),
	}
}
