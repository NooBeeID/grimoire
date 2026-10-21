package base

import (
	"encoding/json"
	"testing"

	"grimoire/lib/common/errs"
	"grimoire/lib/contracts/core/auth/getuser"
)

func reasonOf(err error) string {
	if e, ok := err.(*errs.Error); ok {
		if m, ok := e.Details.(map[string]any); ok {
			s, _ := m["reason"].(string)
			return s
		}
	}
	return ""
}

func TestValidateCandidate(t *testing.T) {
	cases := []struct {
		u    getuser.Response
		want string
	}{
		{getuser.Response{}, ReasonUserNotFound},
		{getuser.Response{Found: true, Role: "super_admin", IsActive: true}, ReasonUserNotDeveloper},
		{getuser.Response{Found: true, Role: "developer"}, ReasonUserInactive},
		{getuser.Response{Found: true, Role: "super_admin"}, ReasonUserNotDeveloper},
		{getuser.Response{Found: true, Role: "developer", IsActive: true}, ""},
	}
	for _, c := range cases {
		err := ValidateCandidate(c.u)
		if c.want == "" && err != nil || c.want != "" && reasonOf(err) != c.want {
			t.Errorf("%+v -> %v", c.u, err)
		}
	}
}

func TestNewMember(t *testing.T) {
	m, err := NewMember("p", "u", "", Actor{ID: "a", Label: "a@x"}, t0)
	if err != nil || m.Role != RoleMember || *m.AddedBy != "a" {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := NewMember("p", "u", "admin", Actor{}, t0); fieldsOf(t, err)["role"] == "" {
		t.Fatal("role invalid harus error")
	}
}

func TestOwnerCountFindMember(t *testing.T) {
	ms := []Member{{UserID: "a", Role: RoleOwner}, {UserID: "b", Role: RoleMember}}
	if OwnerCount(ms) != 1 {
		t.Error("OwnerCount")
	}
	if m, ok := FindMember(ms, "b"); !ok || m.Role != RoleMember {
		t.Error("FindMember")
	}
	if _, ok := FindMember(ms, "z"); ok {
		t.Error("FindMember z")
	}
}

func TestMemberToDTO_NotFound(t *testing.T) {
	b, _ := json.Marshal(Member{UserID: "u", Role: RoleMember, AddedAt: t0}.ToDTO(getuser.Response{}))
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["name"] != nil || m["email"] != nil || m["found"] != false || m["is_active"] != false {
		t.Fatalf("%s", b)
	}
}
