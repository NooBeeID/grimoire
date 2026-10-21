package base

import (
	"encoding/json"
	"testing"
)

func ptr(s string) *string { return &s }

func TestSortMembers(t *testing.T) {
	ms := []MemberDTO{
		{UserID: "5", Role: RoleMember, Name: nil},
		{UserID: "4", Role: RoleMember, Name: ptr("budi")},
		{UserID: "3", Role: RoleMember, Name: ptr("Andi")},
		{UserID: "2", Role: RoleOwner, Name: ptr("Zed")},
		{UserID: "1", Role: RoleMember, Name: ptr("andi")},
	}
	SortMembers(ms)
	got := ""
	for _, m := range ms {
		got += m.UserID
	}
	if got != "21345" {
		t.Fatalf("urutan = %s", got)
	}
}

func TestNewSummaryJSON(t *testing.T) {
	s := NewSummary(Project{ID: "p", Name: "N", CreatedAt: t0, UpdatedAt: t0}, LevelAdmin, "", 0, ContentCounts{})
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["my_role"] != nil || m["is_member"] != false || m["access_level"] != "admin" {
		t.Fatalf("%s", b)
	}
	if c := m["counts"].(map[string]any); c["files"] != nil {
		t.Fatalf("%s", b)
	}
	if s := NewSummary(Project{}, LevelOwner, RoleOwner, 1, ContentCounts{}); s.MyRole == nil || !s.IsMember {
		t.Fatal("anggota")
	}
}
