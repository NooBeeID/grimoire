package usage

import (
	"net/url"
	"testing"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

func TestParseQuery(t *testing.T) {
	q := ParseQuery(url.Values{"range": {" 24h "}, "project_id": {" abc "}, "section": {" keys "}, "x": {"y"}})
	if q.Range != base.Range24h || q.ProjectID != "abc" || q.Section != base.SectionKeys {
		t.Fatalf("%+v", q)
	}
	if q := ParseQuery(url.Values{}); q.Range != "" || q.Section != "" || q.ProjectID != "" {
		t.Fatalf("%+v", q)
	}
}
