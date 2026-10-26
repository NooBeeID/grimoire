package listrecentchanges

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func TestParseSince(t *testing.T) {
	ok := map[string]time.Time{
		"24h":                       now.Add(-24 * time.Hour),
		"7d":                        now.Add(-7 * 24 * time.Hour),
		"90d":                       now.Add(-90 * 24 * time.Hour),
		"2026-10-20T00:00:00Z":      time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		"2026-10-20T07:00:00+07:00": time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
	}
	for in, want := range ok {
		got, e := ParseSince(in, now)
		if e != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("%q -> %v err=%v, want %v", in, got, e, want)
		}
	}
	for _, in := range []string{"91d", "2161h", "2025-01-01T00:00:00Z", "2026-10-27T00:00:00Z", "0h", "1w", "", "-1d", "07d", "9999999d"} {
		_, e := ParseSince(in, now)
		if e == nil || e.Field != "since" {
			t.Errorf("%q harus ditolak dengan Field since, got %v", in, e)
		}
	}
}

func TestParse_Defaults(t *testing.T) {
	p, e := Parse(json.RawMessage(`{"since":"7d"}`), now)
	if e != nil || p.Kind != "all" || p.Limit != 20 || p.Prefix != "" || !p.Since.Equal(now.Add(-7*24*time.Hour)) {
		t.Fatalf("%+v err=%v", p, e)
	}
	p, e = Parse(json.RawMessage(`{"since":"1d","kind":"files","path":"/docs/","limit":50}`), now)
	if e != nil || p.Kind != "files" || p.Prefix != "docs" || p.Limit != 50 {
		t.Fatalf("%+v err=%v", p, e)
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"since hilang", `{}`, "since wajib diisi"},
		{"since terlalu panjang", `{"since":"` + strings.Repeat("1", 65) + `"}`, "since maksimal 64"},
		{"kind salah", `{"since":"1d","kind":"x"}`, "files, specs, all"},
		{"limit 0", `{"since":"1d","limit":0}`, "limit harus 1-50"},
		{"limit 51", `{"since":"1d","limit":51}`, "limit harus 1-50"},
		{"specs + path", `{"since":"1d","kind":"specs","path":"docs"}`, "path hanya berlaku untuk kind files atau all"},
		{"project_id", `{"since":"1d","project_id":"x"}`, "ditentukan oleh API key"},
		{"projectId", `{"since":"1d","projectId":"x"}`, "ditentukan oleh API key"},
		{"project", `{"since":"1d","project":"x"}`, "ditentukan oleh API key"},
		{"asing", `{"since":"1d","foo":1}`, "since, kind, path, limit"},
		{"path dotdot", `{"since":"1d","path":"a/../b"}`, ".."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, e := Parse(json.RawMessage(c.in), now)
			if e == nil || !strings.Contains(e.Message(), c.want) {
				t.Fatalf("err = %v, want memuat %q", e, c.want)
			}
		})
	}
}
