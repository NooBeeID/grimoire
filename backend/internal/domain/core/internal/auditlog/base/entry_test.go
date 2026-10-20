package base

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/audit/record"
)

const (
	uid1 = "0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c"
	uid2 = "0192f0c4-0000-7000-8000-000000000001"
	uid3 = "0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f"
)

var fixedNow = time.Date(2026, 10, 20, 3, 15, 0, 0, time.UTC)

func newID() string { return uid3 }

func validReq() record.Request {
	return record.Request{
		ActorType:    record.ActorAPIKey,
		ActorID:      uid1,
		ActorLabel:   "claude-code-budi (gk_ab12)",
		ProjectID:    uid2,
		ProjectLabel: "Payments",
		Action:       record.ActionMCPReadFile,
		TargetType:   "file",
		Target:       "docs/auth/login.md",
		Args:         map[string]any{"path": "docs/auth/login.md"},
		Result:       map[string]any{"bytes": 4812},
		Status:       record.StatusOK,
		IP:           "203.0.113.5",
		UserAgent:    "claude-code/2.1",
	}
}

func TestNewEntry_Valid(t *testing.T) {
	e, err := NewEntry(validReq(), fixedNow, newID)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != uid3 || !e.OccurredAt.Equal(fixedNow) || e.OccurredAt.Location() != time.UTC {
		t.Fatalf("unexpected %+v", e)
	}
	if e.ActorID != uid1 || e.IP != "203.0.113.5" || e.Args["path"] != "docs/auth/login.md" {
		t.Fatalf("unexpected %+v", e)
	}
}

func TestNewEntry_OccurredAtZoneNormalized(t *testing.T) {
	r := validReq()
	r.OccurredAt = time.Date(2026, 10, 19, 15, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	e, err := NewEntry(r, fixedNow, newID)
	if err != nil {
		t.Fatal(err)
	}
	if e.OccurredAt.Location() != time.UTC || e.OccurredAt.Hour() != 8 {
		t.Fatalf("got %v", e.OccurredAt)
	}
}

func TestNewEntry_ID(t *testing.T) {
	r := validReq()
	r.ID = uid1
	e, err := NewEntry(r, fixedNow, newID)
	if err != nil || e.ID != uid1 {
		t.Fatalf("got %v %v", e.ID, err)
	}
	r.ID = "abc"
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestNewEntry_Action(t *testing.T) {
	cases := map[string]bool{
		"":                                false,
		"MCP.read":                        false,
		"read_file":                       false,
		"mcp..x":                          false,
		"project.member.role_changed":     true,
		"mcp.read_file":                   true,
		"foo.bar":                         true,
		"mcp." + strings.Repeat("a", 130): false,
	}
	for a, ok := range cases {
		r := validReq()
		r.Action = a
		_, err := NewEntry(r, fixedNow, newID)
		if (err == nil) != ok {
			t.Errorf("action %q: ok=%v err=%v", a, ok, err)
		}
		if err != nil && !errors.Is(err, errs.ErrInvalid) {
			t.Errorf("action %q: not ErrInvalid", a)
		}
	}
}

func TestNewEntry_Actor(t *testing.T) {
	r := validReq()
	r.ActorType = "admin"
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatal("actor type admin must be invalid")
	}
	r = validReq()
	r.ActorType, r.ActorID = record.ActorUser, ""
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatal("user without id must be invalid")
	}
	r = validReq()
	r.ActorType, r.ActorLabel = record.ActorUser, "  "
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatal("blank label must be invalid")
	}
	r = validReq()
	r.ActorID = strings.ToUpper(uid1)
	e, err := NewEntry(r, fixedNow, newID)
	if err != nil || e.ActorID != uid1 {
		t.Fatalf("uuid must be canonical lowercase: %v %v", e.ActorID, err)
	}
}

func TestT02_AC2_AnonymousWithKeyPrefix(t *testing.T) {
	r := record.Request{
		ActorType:  record.ActorAnonymous,
		ActorLabel: "gk_ab12",
		Action:     record.ActionMCPAuthRejected,
		Status:     record.StatusDenied,
		Result:     map[string]any{"reason": "invalid_key"},
	}
	e, err := NewEntry(r, fixedNow, newID)
	if err != nil {
		t.Fatal(err)
	}
	if e.ActorID != "" || e.ActorLabel != "gk_ab12" || e.Status != StatusDenied {
		t.Fatalf("unexpected %+v", e)
	}
	r.ActorID = uid1
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatal("anonymous with actor id must be invalid")
	}
	r.ActorID, r.ActorLabel = "", ""
	e, _ = NewEntry(r, fixedNow, newID)
	if e.ActorLabel != "anonymous" {
		t.Fatalf("got %q", e.ActorLabel)
	}
	r.ActorType = record.ActorSystem
	e, _ = NewEntry(r, fixedNow, newID)
	if e.ActorLabel != "system" {
		t.Fatalf("got %q", e.ActorLabel)
	}
}

func TestNewEntry_ProjectAndStatus(t *testing.T) {
	r := validReq()
	r.ProjectID = "x"
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatal("bad project id")
	}
	r.ProjectID = ""
	if _, err := NewEntry(r, fixedNow, newID); err != nil {
		t.Fatal(err)
	}
	r.Status = "failed"
	if _, err := NewEntry(r, fixedNow, newID); !errors.Is(err, errs.ErrInvalid) {
		t.Fatal("bad status")
	}
}

func TestNewEntry_Truncation(t *testing.T) {
	r := validReq()
	r.Target = strings.Repeat("é", 1000)
	e, err := NewEntry(r, fixedNow, newID)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Target) > MaxTargetBytes || !utf8.ValidString(e.Target) {
		t.Fatalf("len=%d valid=%v", len(e.Target), utf8.ValidString(e.Target))
	}
}

func TestNewEntry_IP(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":      "203.0.113.5",
		"::1":              "::1",
		"203.0.113.5:4431": "203.0.113.5",
		"[::1]:80":         "::1",
		"::ffff:10.0.0.1":  "10.0.0.1",
		"not-an-ip":        "",
		"":                 "",
		"fe80::1%eth0":     "fe80::1",
	}
	for in, want := range cases {
		r := validReq()
		r.IP = in
		e, err := NewEntry(r, fixedNow, newID)
		if err != nil {
			t.Fatalf("ip %q: %v", in, err)
		}
		if e.IP != want {
			t.Errorf("ip %q: got %q want %q", in, e.IP, want)
		}
	}
}

func TestNewEntry_NilArgsAndNoLeak(t *testing.T) {
	r := validReq()
	r.Args, r.Result = nil, nil
	e, err := NewEntry(r, fixedNow, newID)
	if err != nil || e.Args == nil || len(e.Args) != 0 || e.Result == nil {
		t.Fatalf("got %+v %v", e, err)
	}
	r = validReq()
	r.ActorID = "super-secret-value"
	_, err = NewEntry(r, fixedNow, newID)
	if err == nil || strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("error must not leak input: %v", err)
	}
}

func TestEntry_SummaryAndDetailJSON(t *testing.T) {
	e, _ := NewEntry(validReq(), fixedNow, newID)
	sb, _ := json.Marshal(e.ToSummary())
	var sm map[string]any
	_ = json.Unmarshal(sb, &sm)
	for _, k := range []string{"args", "result", "ip", "user_agent", "trace_id"} {
		if _, ok := sm[k]; ok {
			t.Errorf("summary must not have %q", k)
		}
	}
	db, _ := json.Marshal(e.ToDetail())
	var dm map[string]any
	_ = json.Unmarshal(db, &dm)
	for _, k := range []string{"id", "occurred_at", "action", "args", "result", "ip", "user_agent", "trace_id"} {
		if _, ok := dm[k]; !ok {
			t.Errorf("detail must have %q", k)
		}
	}

	r := record.Request{ActorType: record.ActorSystem, Action: "job.run", Status: record.StatusOK}
	e, _ = NewEntry(r, fixedNow, newID)
	db, _ = json.Marshal(e.ToDetail())
	_ = json.Unmarshal(db, &dm)
	for _, k := range []string{"actor_id", "project_id", "project_label", "target_type", "target", "ip", "user_agent"} {
		if v, ok := dm[k]; !ok || v != nil {
			t.Errorf("%q must be null, got %v (present=%v)", k, v, ok)
		}
	}
}

func TestEntry_CursorMatches(t *testing.T) {
	e, _ := NewEntry(validReq(), fixedNow, newID)
	c := e.Cursor()
	if c.ID != e.ID || !c.OccurredAt.Equal(e.OccurredAt) {
		t.Fatalf("got %+v", c)
	}
}
