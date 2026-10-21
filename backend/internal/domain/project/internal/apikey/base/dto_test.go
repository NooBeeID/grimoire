package base

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestToDTO_JSON(t *testing.T) {
	loc := time.FixedZone("x", 3600)
	by := "u1"
	rev := time.Date(2026, 10, 23, 12, 0, 0, 0, loc)
	k := Key{ID: "k1", Name: "ci", Prefix: "abcdefgh", SecretHash: HashSecret("s"), CreatedBy: &by,
		CreatedAt: time.Date(2026, 10, 23, 9, 0, 0, 0, loc)}
	d := k.ToDTO()
	if d.Status != StatusActive || d.CreatedAt.Location() != time.UTC {
		t.Fatalf("%+v", d)
	}
	b, _ := json.Marshal(d)
	s := string(b)
	for _, want := range []string{`"status":"active"`, `"last_used_at":null`, `"revoked_at":null`, `"created_at":"2026-10-23T08:00:00Z"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in %s", want, s)
		}
	}
	for _, bad := range []string{"secret", "hash"} {
		if strings.Contains(s, bad) {
			t.Fatalf("leak %s in %s", bad, s)
		}
	}
	k.RevokedAt = &rev
	d = k.ToDTO()
	if d.Status != StatusRevoked || d.RevokedAt.Location() != time.UTC {
		t.Fatalf("%+v", d)
	}
}

func TestResultsJSON(t *testing.T) {
	b, _ := json.Marshal(KeyList{Items: []KeyDTO{}})
	if string(b) != `{"items":[]}` {
		t.Fatal(string(b))
	}
	b, _ = json.Marshal(CreateKeyResult{RawKey: "gr_x"})
	if !strings.Contains(string(b), `"mcp_url":null`) || !strings.Contains(string(b), `"raw_key":"gr_x"`) {
		t.Fatal(string(b))
	}
	b, _ = json.Marshal(KeyEnvelope{})
	if !strings.HasPrefix(string(b), `{"key":{`) {
		t.Fatal(string(b))
	}
}

func TestMCPURL(t *testing.T) {
	if MCPURL("") != nil {
		t.Fatal("nil expected")
	}
	if u := MCPURL("https://g.tim.id"); u == nil || *u != "https://g.tim.id/mcp" {
		t.Fatal(u)
	}
}
