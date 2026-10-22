package summary

import (
	"net/url"
	"testing"

	"grimoire/internal/domain/core/internal/auditlog/base"
)

func TestParseQuery(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want base.SummaryQuery
	}{
		{"trim range", "range=%207d%20", base.SummaryQuery{Range: "7d"}},
		{"kosong", "", base.SummaryQuery{}},
		{"project spasi", "project_id=%20%20", base.SummaryQuery{}},
		{"project", "project_id=abc", base.SummaryQuery{ProjectID: "abc"}},
		{"param asing diabaikan", "foo=bar&action=x&range=24h", base.SummaryQuery{Range: "24h"}},
		{"range berulang", "range=24h&range=30d", base.SummaryQuery{Range: "24h"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := url.ParseQuery(c.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := ParseQuery(v); got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}
